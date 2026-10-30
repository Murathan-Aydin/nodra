package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/Murathan-Aydin/nodra/internal/auth"
	"github.com/Murathan-Aydin/nodra/internal/backup"
	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/deploy"
	"github.com/Murathan-Aydin/nodra/internal/jobs"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Les tests d'intégration nécessitent NODRA_TEST_DATABASE_URL (ex. la base du docker-compose.dev.yml).
// Une base temporaire est créée puis supprimée à chaque exécution.
type testEnv struct {
	srv      *httptest.Server
	mailer   *fakeMailer
	st       *store.Store
	api      *Server
	svc      *servers.Service
	runner   *deploy.Runner
	backups  *backup.Service
	jobs     *jobs.Jobs
	executed []*store.ApprovalRequest
}

func testServer(t *testing.T) (*httptest.Server, *fakeMailer, *store.Store) {
	e := newTestEnv(t)
	return e.srv, e.mailer, e.st
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	base := os.Getenv("NODRA_TEST_DATABASE_URL")
	if base == "" {
		t.Skip("NODRA_TEST_DATABASE_URL non défini")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("nodra_test_%d", rand.Uint32())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(base)
	u.Path = "/" + name
	st, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Migrate(ctx, st.Pool); err != nil {
		t.Fatal(err)
	}
	box, _ := crypto.New(make([]byte, 32))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := ssh.NewPool()
	env := &testEnv{mailer: &fakeMailer{}, st: st, svc: &servers.Service{Store: st, Box: box, SSH: pool, Log: log}}
	env.runner = &deploy.Runner{Store: st, Box: box, SSH: pool, Servers: env.svc, Log: log}
	env.backups = &backup.Service{Store: st, Box: box, SSH: pool, Log: log}
	env.svc.Tasks = env.runner.Tasks()
	for k, v := range env.backups.Tasks() {
		env.svc.Tasks[k] = v
	}
	jq, err := jobs.New(st, env.svc, env.runner, log, jobs.Options{})
	if err != nil {
		t.Fatal(err)
	}
	env.jobs = jq
	if err := jq.Start(ctx); err != nil {
		t.Fatal(err)
	}
	env.api = &Server{
		Store: st, Box: box, Mailer: env.mailer, AuthRateLimit: 1000, Jobs: jq, Log: log, Deployer: env.runner, Backups: env.backups, Targets: env.svc,
		Web: fstest.MapFS{"index.html": {Data: []byte("spa")}},
		Executors: map[string]Executor{"resource.deploy": func(_ context.Context, req *store.ApprovalRequest) error {
			env.executed = append(env.executed, req)
			return nil
		}},
	}
	mux := env.api.Router().(*chi.Mux)
	// Route de test : une action soumise à la porte d'approbation (comme le futur déploiement).
	mux.With(csrfGuard, env.api.authenticate, requireUser).Route("/test/teams/{team}", func(r chi.Router) {
		r.Use(env.api.withTeam)
		r.Post("/deploy", func(w http.ResponseWriter, r *http.Request) {
			if !env.api.authorize(w, r, "resource.deploy", rbac.Scope{}) {
				return
			}
			req, err := env.api.gate(r, "resource.deploy", rbac.Scope{}, "Déploiement de test", map[string]string{"ref": "main"})
			if err != nil {
				env.api.fail(w, err)
				return
			}
			if req != nil {
				respondPending(w, req)
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"executed": true})
		})
	})
	srv := httptest.NewServer(mux)
	env.srv = srv
	t.Cleanup(func() {
		srv.Close()
		stopCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		_ = jq.Stop(stopCtx)
		cancel()
		pool.Close()
		st.Close()
		_, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		admin.Close(ctx)
	})
	return env
}

type fakeMailer struct{ lastToken, lastInvite string }

func (f *fakeMailer) PasswordReset(_ context.Context, _, token string) { f.lastToken = token }
func (f *fakeMailer) TeamInvitation(_ context.Context, _, _, token string) {
	f.lastInvite = token
}

type client struct {
	t      *testing.T
	base   string
	http   *http.Client
	bearer string
}

func newClient(t *testing.T, srv *httptest.Server) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: srv.URL + "/api/v1", http: &http.Client{Jar: jar}}
}

// raw envoie une requête sur une URL absolue (hors /api/v1).
func (c *client) raw(method, url string, body any, out any) int {
	saved := c.base
	c.base = url
	defer func() { c.base = saved }()
	return c.do(method, "", body, out)
}

// do envoie une requête JSON et décode la réponse dans out (si non nil). Renvoie le statut.
func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Requested-With", "nodra-test")
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func expect(t *testing.T, step string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s : statut %d, attendu %d", step, got, want)
	}
}

func TestAuthFlow(t *testing.T) {
	srv, mailer, _ := testServer(t)
	alice := newClient(t, srv)
	bob := newClient(t, srv)

	var u store.User
	expect(t, "inscription alice", alice.do("POST", "/auth/register", map[string]string{"email": "alice@example.com", "password": "motdepasse1", "name": "Alice"}, &u), 201)
	if !u.IsInstanceAdmin {
		t.Fatal("le premier utilisateur doit être administrateur de l'instance")
	}
	expect(t, "inscription bob", bob.do("POST", "/auth/register", map[string]string{"email": "bob@example.com", "password": "motdepasse2"}, &u), 201)
	if u.IsInstanceAdmin {
		t.Fatal("le second utilisateur ne doit pas être administrateur")
	}
	expect(t, "doublon (casse ignorée)", newClient(t, srv).do("POST", "/auth/register", map[string]string{"email": "ALICE@example.com", "password": "motdepasse1"}, nil), 409)
	expect(t, "mot de passe trop court", newClient(t, srv).do("POST", "/auth/register", map[string]string{"email": "c@example.com", "password": "court"}, nil), 422)
	expect(t, "me", alice.do("GET", "/me", nil, nil), 200)
	expect(t, "me anonyme", newClient(t, srv).do("GET", "/me", nil, nil), 401)

	// Connexion
	anon := newClient(t, srv)
	expect(t, "mauvais mot de passe", anon.do("POST", "/auth/login", map[string]string{"email": "alice@example.com", "password": "nope-nope"}, nil), 401)
	expect(t, "utilisateur inconnu", anon.do("POST", "/auth/login", map[string]string{"email": "x@example.com", "password": "nope-nope"}, nil), 401)

	// 2FA
	var setup struct{ Secret string }
	expect(t, "2fa setup", alice.do("POST", "/me/2fa/setup", nil, &setup), 200)
	expect(t, "2fa mauvais code", alice.do("POST", "/me/2fa/enable", map[string]string{"code": "000000"}, nil), 422)
	code, _ := auth.TOTPCode(setup.Secret, time.Now())
	var enabled struct {
		RecoveryCodes []string `json:"recovery_codes"`
	}
	expect(t, "2fa enable", alice.do("POST", "/me/2fa/enable", map[string]string{"code": code}, &enabled), 200)
	if len(enabled.RecoveryCodes) != 8 {
		t.Fatal("8 codes de secours attendus")
	}

	expect(t, "logout", alice.do("POST", "/auth/logout", nil, nil), 204)
	expect(t, "me après logout", alice.do("GET", "/me", nil, nil), 401)

	var login struct {
		MFARequired bool `json:"mfa_required"`
	}
	expect(t, "login alice", alice.do("POST", "/auth/login", map[string]string{"email": "alice@example.com", "password": "motdepasse1"}, &login), 200)
	if !login.MFARequired {
		t.Fatal("la 2FA doit être demandée")
	}
	expect(t, "me avant 2fa", alice.do("GET", "/me", nil, nil), 401)
	expect(t, "2fa login mauvais code", alice.do("POST", "/auth/login/2fa", map[string]string{"code": "000000"}, nil), 401)
	code, _ = auth.TOTPCode(setup.Secret, time.Now())
	expect(t, "2fa login", alice.do("POST", "/auth/login/2fa", map[string]string{"code": code}, nil), 200)
	expect(t, "me après 2fa", alice.do("GET", "/me", nil, nil), 200)

	// Code de secours : utilisable une seule fois.
	other := newClient(t, srv)
	expect(t, "login 2", other.do("POST", "/auth/login", map[string]string{"email": "alice@example.com", "password": "motdepasse1"}, nil), 200)
	expect(t, "code de secours", other.do("POST", "/auth/login/2fa", map[string]string{"recovery_code": enabled.RecoveryCodes[0]}, nil), 200)
	third := newClient(t, srv)
	expect(t, "login 3", third.do("POST", "/auth/login", map[string]string{"email": "alice@example.com", "password": "motdepasse1"}, nil), 200)
	expect(t, "code de secours réutilisé", third.do("POST", "/auth/login/2fa", map[string]string{"recovery_code": enabled.RecoveryCodes[0]}, nil), 401)

	// Sessions : alice voit ses sessions et peut en révoquer une.
	var sessions []struct {
		ID      string
		Current bool
	}
	expect(t, "liste sessions", alice.do("GET", "/me/sessions", nil, &sessions), 200)
	if len(sessions) != 2 {
		t.Fatalf("2 sessions actives attendues, %d trouvées", len(sessions))
	}
	for _, s := range sessions {
		if !s.Current {
			expect(t, "révocation session", alice.do("DELETE", "/me/sessions/"+s.ID, nil, nil), 204)
		}
	}
	expect(t, "session révoquée", other.do("GET", "/me", nil, nil), 401)

	// Jetons API
	var created struct {
		Token    string
		APIToken store.APIToken `json:"api_token"`
	}
	expect(t, "scope inconnu", bob.do("POST", "/me/tokens", map[string]any{"name": "ci", "scopes": []string{"admin"}}, nil), 422)
	expect(t, "création jeton", bob.do("POST", "/me/tokens", map[string]any{"name": "ci", "scopes": []string{"read", "deploy"}}, &created), 201)
	api := newClient(t, srv)
	api.bearer = created.Token
	expect(t, "me via jeton", api.do("GET", "/me", nil, nil), 200)
	expect(t, "gestion compte via jeton", api.do("GET", "/me/tokens", nil, nil), 403)
	expect(t, "suppression jeton d'un autre", alice.do("DELETE", "/me/tokens/"+created.APIToken.ID, nil, nil), 404)
	expect(t, "suppression jeton", bob.do("DELETE", "/me/tokens/"+created.APIToken.ID, nil, nil), 204)
	expect(t, "jeton supprimé", api.do("GET", "/me", nil, nil), 401)

	// Mot de passe oublié : réponse identique que le compte existe ou non.
	expect(t, "forgot inconnu", anon.do("POST", "/auth/password/forgot", map[string]string{"email": "x@example.com"}, nil), 204)
	expect(t, "forgot bob", anon.do("POST", "/auth/password/forgot", map[string]string{"email": "bob@example.com"}, nil), 204)
	if mailer.lastToken == "" {
		t.Fatal("un jeton de réinitialisation doit être envoyé")
	}
	expect(t, "reset", anon.do("POST", "/auth/password/reset", map[string]string{"token": mailer.lastToken, "password": "nouveaumdp1"}, nil), 204)
	expect(t, "reset rejoué", anon.do("POST", "/auth/password/reset", map[string]string{"token": mailer.lastToken, "password": "nouveaumdp2"}, nil), 400)
	expect(t, "anciennes sessions révoquées", bob.do("GET", "/me", nil, nil), 401)
	expect(t, "login nouveau mdp", bob.do("POST", "/auth/login", map[string]string{"email": "bob@example.com", "password": "nouveaumdp1"}, nil), 200)

	// Changement de mot de passe
	expect(t, "changement mauvais actuel", bob.do("POST", "/me/password", map[string]string{"current_password": "faux-faux", "new_password": "encoreunautre"}, nil), 401)
	expect(t, "changement", bob.do("POST", "/me/password", map[string]string{"current_password": "nouveaumdp1", "new_password": "encoreunautre"}, nil), 204)
	expect(t, "session courante conservée", bob.do("GET", "/me", nil, nil), 200)
}

func TestCSRFGuard(t *testing.T) {
	srv, _, _ := testServer(t)
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/auth/login", bytes.NewBufferString("email=a&password=b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	expect(t, "formulaire cross-site", resp.StatusCode, 415)

	req, _ = http.NewRequest("POST", srv.URL+"/api/v1/auth/logout", nil)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	expect(t, "POST sans corps ni X-Requested-With", resp.StatusCode, 403)
}

func TestRegistrationClosed(t *testing.T) {
	srv, _, st := testServer(t)
	if _, err := st.Pool.Exec(context.Background(), `UPDATE instance_settings SET registration_enabled = false`); err != nil {
		t.Fatal(err)
	}
	// Le premier compte (admin) reste toujours possible, même inscriptions fermées.
	expect(t, "premier", newClient(t, srv).do("POST", "/auth/register", map[string]string{"email": "a@example.com", "password": "motdepasse1"}, nil), 201)
	expect(t, "second refusé", newClient(t, srv).do("POST", "/auth/register", map[string]string{"email": "b@example.com", "password": "motdepasse1"}, nil), 403)
}

func TestRateLimiter(t *testing.T) {
	l := newRateLimiter(2, time.Minute)
	if !l.allow("ip") || !l.allow("ip") || l.allow("ip") {
		t.Fatal("la 3e tentative doit être bloquée")
	}
	if !l.allow("autre") {
		t.Fatal("une autre IP ne doit pas être bloquée")
	}
}

func jsonDecode(r io.Reader, v any) error { return json.NewDecoder(r).Decode(v) }
