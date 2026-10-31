package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

// fakeIdP : fournisseur OIDC (découverte) et GitHub Enterprise simulés.
type fakeIdP struct {
	mu        sync.Mutex
	challenge string
	claims    map[string]any // OIDC
	github    [2]any         // utilisateur, e-mails
	srv       *httptest.Server
}

func newFakeIdP(t *testing.T) *fakeIdP {
	f := &fakeIdP{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			reply(map[string]string{"authorization_endpoint": f.srv.URL + "/authorize", "token_endpoint": f.srv.URL + "/token",
				"userinfo_endpoint": f.srv.URL + "/userinfo"})
		case "/token", "/login/oauth/access_token":
			_ = r.ParseForm()
			sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "bon-code" || base64.RawURLEncoding.EncodeToString(sum[:]) != f.challenge || r.Form.Get("client_secret") != "s3cret" {
				w.WriteHeader(http.StatusBadRequest)
				reply(map[string]string{"error": "invalid_grant"})
				return
			}
			reply(map[string]string{"access_token": "jeton", "token_type": "bearer"})
		case "/userinfo":
			reply(f.claims)
		case "/api/v3/user":
			reply(f.github[0])
		case "/api/v3/user/emails":
			reply(f.github[1])
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// oauthLogin suit le flux : démarrage, autorisation simulée, retour ; renvoie la redirection finale.
func oauthLogin(t *testing.T, c *client, idp *fakeIdP, provider, query string) string {
	t.Helper()
	c.http.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { c.http.CheckRedirect = nil }()
	req, _ := http.NewRequest("GET", c.base+"/auth/oauth/"+provider+"/start?"+query, nil)
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || loc.Query().Get("state") == "" {
		return resp.Header.Get("Location")
	}
	if !strings.HasPrefix(loc.String(), idp.srv.URL) || loc.Query().Get("code_challenge_method") != "S256" ||
		!strings.HasSuffix(loc.Query().Get("redirect_uri"), "/api/v1/auth/oauth/"+provider+"/callback") {
		t.Fatalf("autorisation : %s", loc)
	}
	idp.mu.Lock()
	idp.challenge = loc.Query().Get("code_challenge")
	idp.mu.Unlock()
	cb := c.base + "/auth/oauth/" + provider + "/callback?code=bon-code&state=" + url.QueryEscape(loc.Query().Get("state"))
	resp, err = c.http.Get(cb)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("retour : %d", resp.StatusCode)
	}
	// Rejouer le même état est refusé.
	if again, _ := c.http.Get(cb); again != nil {
		again.Body.Close()
		if !strings.Contains(again.Header.Get("Location"), "oauth_error=") {
			t.Fatalf("état rejoué accepté : %s", again.Header.Get("Location"))
		}
	}
	return resp.Header.Get("Location")
}

func TestOAuth(t *testing.T) {
	e := newTestEnv(t)
	idp := newFakeIdP(t)
	alice, _ := register(t, e, "alice@example.com")
	bob, _ := register(t, e, "bob@example.com")

	expect(t, "fournisseur inconnu", alice.do("PUT", "/admin/oauth/myspace", map[string]any{}, nil), 404)
	expect(t, "secret manquant", alice.do("PUT", "/admin/oauth/oidc", map[string]any{"enabled": true, "client_id": "nodra", "base_url": idp.srv.URL}, nil), 422)
	expect(t, "oidc", alice.do("PUT", "/admin/oauth/oidc", map[string]any{"enabled": true, "client_id": "nodra", "client_secret": "s3cret",
		"base_url": idp.srv.URL + "/"}, nil), 204)
	expect(t, "github", alice.do("PUT", "/admin/oauth/github", map[string]any{"enabled": true, "client_id": "gh", "client_secret": "s3cret",
		"base_url": idp.srv.URL}, nil), 204)
	expect(t, "secret conservé", alice.do("PUT", "/admin/oauth/github", map[string]any{"enabled": true, "client_id": "gh", "client_secret": "********",
		"base_url": idp.srv.URL}, nil), 204)
	var settings []map[string]any
	expect(t, "réglages", alice.do("GET", "/admin/oauth", nil, &settings), 200)
	if len(settings) != 6 || settings[0]["provider"] != "github" || settings[0]["client_secret"] != "********" ||
		!strings.HasSuffix(settings[0]["redirect_uri"].(string), "/api/v1/auth/oauth/github/callback") {
		t.Fatalf("réglages : %v", settings)
	}
	var inst struct {
		OAuthProviders []map[string]string `json:"oauth_providers"`
	}
	expect(t, "instance", newClient(t, e.srv).do("GET", "/instance", nil, &inst), 200)
	if len(inst.OAuthProviders) != 2 {
		t.Fatalf("fournisseurs proposés : %v", inst.OAuthProviders)
	}

	// Nouveau compte créé par OIDC (adresse vérifiée par le fournisseur).
	carol := newClient(t, e.srv)
	idp.claims = map[string]any{"sub": "c-1", "email": "carol@example.com", "email_verified": true, "name": "Carol"}
	if loc := oauthLogin(t, carol, idp, "oidc", "redirect=/projects"); loc != "/projects" {
		t.Fatalf("connexion : %s", loc)
	}
	var me struct{ User store.User }
	expect(t, "me", carol.do("GET", "/me", nil, &me), 200)
	if me.User.Email != "carol@example.com" || me.User.Name != "Carol" || me.User.EmailVerifiedAt == nil || me.User.IsInstanceAdmin {
		t.Fatalf("compte créé : %+v", me.User)
	}
	expect(t, "pas de mot de passe", newClient(t, e.srv).do("POST", "/auth/login", map[string]string{"email": "carol@example.com", "password": ""}, nil), 401)
	if loc := oauthLogin(t, newClient(t, e.srv), idp, "oidc", "redirect=//evil.example"); loc != "/" {
		t.Fatalf("redirection ouverte : %s", loc)
	}

	// Compte existant : rattaché seulement si le fournisseur a vérifié l'adresse.
	idp.claims = map[string]any{"sub": "b-1", "email": "bob@example.com", "email_verified": false}
	if loc := oauthLogin(t, newClient(t, e.srv), idp, "oidc", ""); !strings.Contains(loc, "oauth_error=") || !strings.HasPrefix(loc, "/login") {
		t.Fatalf("adresse non vérifiée : %s", loc)
	}
	idp.claims["email_verified"] = "true"
	bob2 := newClient(t, e.srv)
	if loc := oauthLogin(t, bob2, idp, "oidc", ""); loc != "/" {
		t.Fatalf("rattachement : %s", loc)
	}
	expect(t, "me bob", bob2.do("GET", "/me", nil, &me), 200)
	if me.User.Email != "bob@example.com" {
		t.Fatalf("bob : %+v", me.User)
	}

	// Liaison depuis le profil (GitHub Enterprise).
	idp.github = [2]any{map[string]any{"id": 42, "login": "alice-gh"}, []map[string]any{{"email": "alice@corp.example", "primary": true, "verified": true}}}
	if loc := oauthLogin(t, alice, idp, "github", "link=1&redirect=/settings/account"); loc != "/settings/account?oauth_linked=github" {
		t.Fatalf("liaison : %s", loc)
	}
	var ids []store.Identity
	expect(t, "identités", alice.do("GET", "/me/identities", nil, &ids), 200)
	if len(ids) != 1 || ids[0].Provider != "github" || ids[0].Subject != "42" || ids[0].Email != "alice@corp.example" {
		t.Fatalf("identités : %+v", ids)
	}
	if loc := oauthLogin(t, carol, idp, "github", "link=1&redirect=/settings/account"); !strings.HasPrefix(loc, "/settings/account?oauth_error=") {
		t.Fatalf("compte déjà lié : %s", loc)
	}
	if loc := oauthLogin(t, newClient(t, e.srv), idp, "github", "link=1"); !strings.HasPrefix(loc, "/login?oauth_error=") {
		t.Fatalf("liaison sans session : %s", loc)
	}
	githubLogin := newClient(t, e.srv)
	if loc := oauthLogin(t, githubLogin, idp, "github", ""); loc != "/" {
		t.Fatalf("connexion GitHub : %s", loc)
	}
	expect(t, "me via github", githubLogin.do("GET", "/me", nil, &me), 200)
	if me.User.Email != "alice@example.com" {
		t.Fatalf("identité liée : %+v", me.User)
	}

	// Inscriptions fermées : pas de nouveau compte.
	expect(t, "inscriptions fermées", alice.do("PATCH", "/admin/settings", map[string]any{"registration_enabled": false}, nil), 200)
	idp.claims = map[string]any{"sub": "d-1", "email": "dave@example.com", "email_verified": true}
	if loc := oauthLogin(t, newClient(t, e.srv), idp, "oidc", ""); !strings.Contains(loc, "inscriptions") {
		t.Fatalf("inscriptions fermées : %s", loc)
	}

	// Déliaison : jamais le dernier moyen de connexion.
	expect(t, "dernier moyen", carol.do("DELETE", "/me/identities/oidc", nil, nil), 409)
	expect(t, "déliaison", bob.do("DELETE", "/me/identities/oidc", nil, nil), 204)
	expect(t, "déjà déliée", bob.do("DELETE", "/me/identities/oidc", nil, nil), 404)

	// Compte désactivé.
	var users []store.AdminUser
	expect(t, "utilisateurs", alice.do("GET", "/admin/users?search=carol", nil, &users), 200)
	expect(t, "désactivation", alice.do("PATCH", "/admin/users/"+users[0].ID, map[string]any{"disabled": true}, nil), 200)
	idp.claims = map[string]any{"sub": "c-1", "email": "carol@example.com", "email_verified": true}
	if loc := oauthLogin(t, newClient(t, e.srv), idp, "oidc", ""); !strings.Contains(loc, "d%C3%A9sactiv") {
		t.Fatalf("compte désactivé : %s", loc)
	}
	expect(t, "fournisseur désactivé", alice.do("PUT", "/admin/oauth/oidc", map[string]any{"enabled": false, "client_id": "nodra"}, nil), 204)
	if loc := oauthLogin(t, newClient(t, e.srv), idp, "oidc", ""); !strings.Contains(loc, "indisponible") {
		t.Fatalf("fournisseur désactivé : %s", loc)
	}
}
