package api

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/git"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

type captureEvents struct{ events []*git.Event }

func (c *captureEvents) HandleGitEvent(_ *http.Request, _ *store.GitSource, ev *git.Event) (any, error) {
	c.events = append(c.events, ev)
	return map[string]int{"deployments": 0}, nil
}

// fakeGHE simule un GitHub Enterprise (html_url = srv, api_url = srv/api/v3).
func fakeGHE(t *testing.T, pemKey string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/api/v3/app-manifests/CODE/conversions":
			b, _ := json.Marshal(map[string]any{"id": 4242, "slug": "nodra-acme", "client_id": "Iv1.x", "client_secret": "cs",
				"webhook_secret": "whsec", "pem": pemKey, "owner": map[string]string{"login": "acme"}})
			w.Write(b)
		case r.Method == "POST" && r.URL.Path == "/api/v3/app/installations/77/access_tokens":
			fmt.Fprintf(w, `{"token":"ghs_x","expires_at":%q}`, time.Now().Add(time.Hour).Format(time.RFC3339))
		case r.URL.Path == "/api/v3/installation/repositories":
			fmt.Fprint(w, `{"repositories":[{"full_name":"acme/web","name":"web","private":true,"default_branch":"main"}]}`)
		case r.URL.Path == "/api/v3/repos/acme/web/branches":
			fmt.Fprint(w, `[{"name":"main"}]`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSources(t *testing.T) {
	e := newTestEnv(t)
	alice, _ := register(t, e, "alice@example.com")
	bob, _ := register(t, e, "bob@example.com")
	var acme store.Team
	expect(t, "Acme", alice.do("POST", "/teams", map[string]string{"name": "Acme"}, &acme), 201)
	invite(t, e, alice, bob, acme.ID, "bob@example.com", rbac.RoleDeveloper)
	base := "/teams/" + acme.ID
	k, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemKey := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)}))
	ghe := fakeGHE(t, pemKey)

	// Création par manifeste (GitHub Enterprise simulé).
	expect(t, "manifeste par développeur", bob.do("POST", base+"/sources/github/manifest", map[string]string{"name": "x"}, nil), 403)
	var started struct {
		Source     map[string]any `json:"source"`
		FormAction string         `json:"form_action"`
		Manifest   string         `json:"manifest"`
	}
	expect(t, "manifeste", alice.do("POST", base+"/sources/github/manifest", map[string]string{"name": "GitHub Acme", "html_url": ghe.URL}, &started), 201)
	sid := started.Source["id"].(string)
	if started.Source["configured"] != false || !strings.HasPrefix(started.FormAction, ghe.URL+"/settings/apps/new?state=nst_") {
		t.Fatalf("démarrage : %+v", started)
	}
	var manifest map[string]any
	_ = json.Unmarshal([]byte(started.Manifest), &manifest)
	if !strings.HasSuffix(manifest["redirect_url"].(string), "/api/v1/sources/github/callback") || !strings.HasPrefix(manifest["name"].(string), "nodra-github-acme-") {
		t.Fatalf("manifeste : %v", manifest)
	}
	state := started.FormAction[strings.Index(started.FormAction, "state=")+6:]
	state, _ = url.QueryUnescape(state)

	noRedirect := func(c *client) {
		c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	noRedirect(alice)
	noRedirect(bob)
	expect(t, "callback par un non-gestionnaire", bob.do("GET", "/sources/github/callback?code=CODE&state="+url.QueryEscape(state), nil, nil), 403)
	expect(t, "callback état invalide", alice.do("GET", "/sources/github/callback?code=CODE&state=faux", nil, nil), 400)
	req, _ := http.NewRequest("GET", alice.base+"/sources/github/callback?code=CODE&state="+url.QueryEscape(state), nil)
	resp, err := alice.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc := resp.Header.Get("Location")
	if resp.StatusCode != 302 || !strings.HasPrefix(loc, ghe.URL+"/github-apps/nodra-acme/installations/new?state=") {
		t.Fatalf("redirection vers l'installation : %d %s", resp.StatusCode, loc)
	}
	expect(t, "état à usage unique", alice.do("GET", "/sources/github/callback?code=CODE&state="+url.QueryEscape(state), nil, nil), 400)
	state2, _ := url.QueryUnescape(loc[strings.Index(loc, "state=")+6:])
	req, _ = http.NewRequest("GET", alice.base+"/sources/github/install?installation_id=77&setup_action=install&state="+url.QueryEscape(state2), nil)
	resp, _ = alice.http.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 302 || resp.Header.Get("Location") != "/t/"+acme.ID+"/sources/"+sid+"?installed=1" {
		t.Fatalf("installation : %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	var src map[string]any
	expect(t, "source", bob.do("GET", base+"/sources/"+sid, nil, &src), 200)
	if src["app_id"].(float64) != 4242 || src["installation_id"].(float64) != 77 || src["installed"] != true || src["has_private_key"] != true {
		t.Fatalf("source après installation : %v", src)
	}
	if _, leaked := src["private_key"]; leaked || strings.Contains(fmt.Sprint(src), "whsec") {
		t.Fatal("aucun secret ne doit être renvoyé")
	}

	// Dépôts et branches via l'API (jeton d'installation).
	var repos []git.Repo
	expect(t, "dépôts", bob.do("GET", base+"/sources/"+sid+"/repositories", nil, &repos), 200)
	if len(repos) != 1 || repos[0].FullName != "acme/web" {
		t.Fatalf("dépôts : %+v", repos)
	}
	var branches []string
	expect(t, "branches", bob.do("GET", base+"/sources/"+sid+"/branches?repo=acme/web", nil, &branches), 200)
	expect(t, "dépôt invalide", bob.do("GET", base+"/sources/"+sid+"/branches?repo=../x", nil, nil), 502)

	// Webhook de l'App : signature vérifiée avec le secret reçu de GitHub.
	capture := &captureEvents{}
	e.api.GitEvents = capture
	hook := func(event string, body []byte, secret string) int {
		m := hmac.New(sha256.New, []byte(secret))
		m.Write(body)
		req, _ := http.NewRequest("POST", e.srv.URL+"/webhooks/github", strings.NewReader(string(body)))
		req.Header.Set("X-GitHub-Event", event)
		req.Header.Set("X-GitHub-Hook-Installation-Target-ID", "4242")
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(m.Sum(nil)))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	push := []byte(`{"ref":"refs/heads/main","after":"abc","repository":{"full_name":"acme/web"},"installation":{"id":77}}`)
	expect(t, "webhook mauvaise signature", hook("push", push, "autre"), 401)
	expect(t, "webhook push", hook("push", push, "whsec"), 200)
	if len(capture.events) != 1 || capture.events[0].Branch != "main" {
		t.Fatalf("événement transmis : %+v", capture.events)
	}
	expect(t, "ping", hook("ping", []byte(`{"zen":"x"}`), "whsec"), 200)
	expect(t, "désinstallation", hook("installation", []byte(`{"action":"deleted","installation":{"id":77}}`), "whsec"), 200)
	expect(t, "source", bob.do("GET", base+"/sources/"+sid, nil, &src), 200)
	if src["installed"] != false {
		t.Fatal("la désinstallation doit effacer installation_id")
	}

	// Sources GitLab / Gitea saisies manuellement.
	expect(t, "gitlab sans jeton", alice.do("POST", base+"/sources", map[string]any{"type": "gitlab", "name": "GitLab"}, nil), 422)
	expect(t, "gitea sans URL", alice.do("POST", base+"/sources", map[string]any{"type": "gitea", "name": "Gitea", "access_token": "t"}, nil), 422)
	expect(t, "URL avec identifiants", alice.do("POST", base+"/sources", map[string]any{"type": "gitea", "name": "G", "html_url": "https://u:p@git.example.com", "access_token": "t"}, nil), 422)
	var gl map[string]any
	expect(t, "gitlab", alice.do("POST", base+"/sources", map[string]any{"type": "gitlab", "name": "GitLab", "access_token": "glpat-x"}, &gl), 201)
	if gl["api_url"] != "https://gitlab.com/api/v4" || gl["has_access_token"] != true {
		t.Fatalf("gitlab : %v", gl)
	}
	expect(t, "github manuel incomplet", alice.do("POST", base+"/sources", map[string]any{"type": "github", "name": "GH", "app_id": 1}, nil), 422)
	expect(t, "clé GitHub invalide", alice.do("POST", base+"/sources", map[string]any{"type": "github", "name": "GH", "app_id": 1, "webhook_secret": "w", "private_key": "x"}, nil), 422)
	expect(t, "github manuel", alice.do("POST", base+"/sources", map[string]any{"type": "github", "name": "GH", "app_id": 5, "installation_id": 6, "webhook_secret": "w", "private_key": pemKey}, nil), 201)
	var list []map[string]any
	expect(t, "liste", bob.do("GET", base+"/sources", nil, &list), 200)
	if len(list) != 3 {
		t.Fatalf("3 sources attendues : %d", len(list))
	}
	expect(t, "renommage", alice.do("PATCH", base+"/sources/"+gl["id"].(string), map[string]any{"name": "GitLab.com"}, nil), 200)
	expect(t, "suppression", alice.do("DELETE", base+"/sources/"+gl["id"].(string), nil, nil), 204)
}
