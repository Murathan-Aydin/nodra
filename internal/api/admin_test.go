package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/jobs"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestInstanceAdmin(t *testing.T) {
	e := newTestEnv(t)
	alice, aliceID := register(t, e, "alice@example.com")
	bob, bobID := register(t, e, "bob@example.com")
	anon := newClient(t, e.srv)

	var pub map[string]any
	expect(t, "instance publique", anon.do("GET", "/instance", nil, &pub), 200)
	if pub["name"] != "Nodra" || pub["registration_enabled"] != true || pub["version"] != "dev" {
		t.Fatalf("instance : %v", pub)
	}
	expect(t, "non-admin", bob.do("GET", "/admin/settings", nil, nil), 403)

	for name, body := range map[string]map[string]any{
		"fqdn":     {"fqdn": "ftp://nodra.example.com"},
		"chemin":   {"fqdn": "https://nodra.example.com/app"},
		"fuseau":   {"timezone": "Mars/Olympus"},
		"cron":     {"update_check_frequency": "souvent"},
		"ipv4":     {"public_ipv4": "1.2.3"},
		"ipv6":     {"public_ipv6": "1.2.3.4"},
		"ip api":   {"api_allowed_ips": []string{"abc"}},
		"dns":      {"dns_servers": []string{"dns.google"}},
		"nom vide": {"instance_name": " "},
	} {
		expect(t, name, alice.do("PATCH", "/admin/settings", body, nil), 422)
	}
	var view struct {
		Settings        store.InstanceSettings `json:"settings"`
		UpdateAvailable bool                   `json:"update_available"`
	}
	expect(t, "réglages", alice.do("PATCH", "/admin/settings", map[string]any{"fqdn": "Nodra.Example.com", "instance_name": "Acme PaaS",
		"timezone": "Europe/Paris", "dns_servers": []string{"1.1.1.1", "9.9.9.9:53"}, "api_allowed_ips": []string{"10.1.2.3/8"},
		"public_ipv4": "203.0.113.10"}, &view), 200)
	st := view.Settings
	if *st.FQDN != "https://nodra.example.com" || st.InstanceName != "Acme PaaS" || st.Timezone != "Europe/Paris" ||
		len(st.DNSServers) != 2 || st.APIAllowedIPs[0] != "10.0.0.0/8" || !st.RegistrationEnabled {
		t.Fatalf("réglages : %+v", st)
	}
	expect(t, "instance publique", anon.do("GET", "/instance", nil, &pub), 200)
	if pub["name"] != "Acme PaaS" {
		t.Fatalf("nom : %v", pub)
	}

	// Accès par jeton : IP autorisées, API désactivable ; les sessions ne sont pas concernées.
	var tok struct {
		Token string `json:"token"`
	}
	expect(t, "jeton", alice.do("POST", "/me/tokens", map[string]any{"name": "ci", "scopes": []string{"root"}}, &tok), 201)
	api := newClient(t, e.srv)
	api.bearer = tok.Token
	expect(t, "IP non autorisée", api.do("GET", "/me", nil, nil), 403)
	expect(t, "IP locale autorisée", alice.do("PATCH", "/admin/settings", map[string]any{"api_allowed_ips": []string{"127.0.0.1", "::1"}}, nil), 200)
	expect(t, "jeton accepté", api.do("GET", "/me", nil, nil), 200)
	expect(t, "API désactivée", alice.do("PATCH", "/admin/settings", map[string]any{"api_enabled": false}, nil), 200)
	expect(t, "jeton refusé", api.do("GET", "/me", nil, nil), 403)
	expect(t, "session acceptée", alice.do("GET", "/me", nil, nil), 200)
	expect(t, "API réactivée", alice.do("PATCH", "/admin/settings", map[string]any{"api_enabled": true, "api_allowed_ips": []string{}}, nil), 200)

	// Utilisateurs.
	var users []store.AdminUser
	expect(t, "utilisateurs", alice.do("GET", "/admin/users", nil, &users), 200)
	if len(users) != 2 || users[0].Email != "bob@example.com" || users[0].Teams != 1 {
		t.Fatalf("utilisateurs : %+v", users)
	}
	expect(t, "recherche", alice.do("GET", "/admin/users?search=ALI", nil, &users), 200)
	if len(users) != 1 || users[0].ID != aliceID {
		t.Fatalf("recherche : %+v", users)
	}
	expect(t, "soi-même", alice.do("PATCH", "/admin/users/"+aliceID, map[string]any{"is_instance_admin": false}, nil), 409)
	expect(t, "promotion", alice.do("PATCH", "/admin/users/"+bobID, map[string]any{"is_instance_admin": true}, nil), 200)
	expect(t, "bob admin", bob.do("GET", "/admin/settings", nil, nil), 200)
	expect(t, "rétrogradation", alice.do("PATCH", "/admin/users/"+bobID, map[string]any{"is_instance_admin": false}, nil), 200)
	expect(t, "bob n'est plus admin", bob.do("GET", "/admin/settings", nil, nil), 403)

	var bobTok struct {
		Token string `json:"token"`
	}
	expect(t, "jeton de bob", bob.do("POST", "/me/tokens", map[string]any{"name": "ci", "scopes": []string{"read"}}, &bobTok), 201)
	bobAPI := newClient(t, e.srv)
	bobAPI.bearer = bobTok.Token
	expect(t, "désactivation", alice.do("PATCH", "/admin/users/"+bobID, map[string]any{"disabled": true}, nil), 200)
	expect(t, "session révoquée", bob.do("GET", "/me", nil, nil), 401)
	expect(t, "jeton refusé", bobAPI.do("GET", "/me", nil, nil), 401)
	login := map[string]string{"email": "bob@example.com", "password": "motdepasse1"}
	expect(t, "connexion refusée", bob.do("POST", "/auth/login", login, nil), 403)
	expect(t, "réactivation", alice.do("PATCH", "/admin/users/"+bobID, map[string]any{"disabled": false}, nil), 200)
	expect(t, "connexion", bob.do("POST", "/auth/login", login, nil), 200)
	expect(t, "déconnexion forcée", alice.do("POST", "/admin/users/"+bobID+"/logout", nil, nil), 204)
	expect(t, "bob déconnecté", bob.do("GET", "/me", nil, nil), 401)

	// Journal de l'instance.
	var audit struct{ Events []store.AuditEvent }
	expect(t, "audit instance", alice.do("GET", "/admin/audit?action=instance.", nil, &audit), 200)
	if len(audit.Events) < 5 || audit.Events[0].Action != "instance.user.logged_out" || audit.Events[0].TeamID != nil {
		t.Fatalf("audit : %+v", audit.Events)
	}
	expect(t, "audit connexions", alice.do("GET", "/admin/audit?action=auth.login.disabled", nil, &audit), 200)
	if len(audit.Events) != 1 {
		t.Fatalf("connexion refusée journalisée : %+v", audit.Events)
	}
	expect(t, "audit non-admin", bob.do("GET", "/admin/audit", nil, nil), 401)

	// Mises à jour : vérification immédiate puis selon la fréquence.
	releases := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name": "v99.0.0"}`))
	}))
	defer releases.Close()
	e.api.ReleasesAPI = releases.URL
	var check map[string]any
	expect(t, "vérification", alice.do("POST", "/admin/update-check", nil, &check), 200)
	if check["latest_version"] != "v99.0.0" || check["update_available"] != false { // version de développement
		t.Fatalf("vérification : %v", check)
	}
	ctx := context.Background()
	if done, err := jobs.CheckUpdates(ctx, e.st, releases.URL, time.Now()); err != nil || done {
		t.Fatalf("vérification trop récente : %v %v", done, err)
	}
	if done, err := jobs.CheckUpdates(ctx, e.st, releases.URL, time.Now().Add(2*time.Hour)); err != nil || !done {
		t.Fatalf("vérification due : %v %v", done, err)
	}
}

func TestIPAllowed(t *testing.T) {
	list := []string{"10.0.0.0/8", "192.168.1.5", "2001:db8::/32"}
	for ip, want := range map[string]bool{"10.2.3.4": true, "192.168.1.5": true, "::ffff:192.168.1.5": true, "192.168.1.6": false,
		"2001:db8::1": true, "nope": false} {
		if got := ipAllowed(ip, list); got != want {
			t.Errorf("ipAllowed(%s) = %v", ip, got)
		}
	}
}
