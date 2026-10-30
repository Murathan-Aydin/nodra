package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/notify"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// hook enregistre les requêtes reçues (webhook de notification ou API Resend).
type hook struct {
	mu     sync.Mutex
	bodies []map[string]any
	auth   []string
	status int
	srv    *httptest.Server
}

func newHook(t *testing.T) *hook {
	h := &hook{status: http.StatusOK}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m map[string]any
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &m)
		h.mu.Lock()
		h.bodies = append(h.bodies, m)
		h.auth = append(h.auth, r.Header.Get("Authorization"))
		status := h.status
		h.mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *hook) received() []map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]map[string]any(nil), h.bodies...)
}

func (h *hook) setStatus(code int) {
	h.mu.Lock()
	h.status = code
	h.mu.Unlock()
}

// wait attend au moins n requêtes.
func (h *hook) wait(t *testing.T, n int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if got := h.received(); len(got) >= n {
			return got
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("%d requêtes attendues, %d reçues", n, len(h.received()))
	return nil
}

func TestNotificationChannels(t *testing.T) {
	e := newTestEnv(t)
	alice, _ := register(t, e, "alice@example.com")
	bob, _ := register(t, e, "bob@example.com")
	var acme store.Team
	expect(t, "Acme", alice.do("POST", "/teams", map[string]string{"name": "Acme"}, &acme), 201)
	base := "/teams/" + acme.ID
	invite(t, e, alice, bob, acme.ID, "bob@example.com", rbac.RoleViewer)

	var catalog struct {
		Events       []map[string]any `json:"events"`
		ChannelTypes []string         `json:"channel_types"`
	}
	expect(t, "catalogue", bob.do("GET", "/notifications/catalog", nil, &catalog), 200)
	if len(catalog.Events) < 10 || len(catalog.ChannelTypes) != 6 {
		t.Fatalf("catalogue : %+v", catalog)
	}

	h := newHook(t)
	ch := map[string]any{"type": "webhook", "name": "Ops", "config": map[string]any{"webhook_url": h.srv.URL + "/secret-path"}}
	expect(t, "lecteur sans notification.manage", bob.do("POST", base+"/notification-channels", ch, nil), 403)
	expect(t, "type inconnu", alice.do("POST", base+"/notification-channels", map[string]any{"type": "fax", "name": "x"}, nil), 422)
	expect(t, "URL manquante", alice.do("POST", base+"/notification-channels", map[string]any{"type": "slack", "name": "x"}, nil), 422)
	expect(t, "événement inconnu", alice.do("POST", base+"/notification-channels",
		map[string]any{"type": "webhook", "name": "x", "config": ch["config"], "events": []string{"nope"}}, nil), 422)
	var created struct {
		ID     string        `json:"id"`
		Events []string      `json:"events"`
		Config notify.Config `json:"config"`
	}
	expect(t, "canal webhook", alice.do("POST", base+"/notification-channels", ch, &created), 201)
	if created.Config.WebhookURL != "********" || len(created.Events) == 0 {
		t.Fatalf("canal : %+v", created)
	}
	var list []map[string]any
	expect(t, "liste par un lecteur", bob.do("GET", base+"/notification-channels", nil, &list), 200)
	if len(list) != 1 || strings.Contains(toJSON(list), "secret-path") {
		t.Fatalf("liste : %s", toJSON(list))
	}

	// Secret masqué renvoyé tel quel : l'URL est conservée.
	expect(t, "abonnements", alice.do("PATCH", base+"/notification-channels/"+created.ID,
		map[string]any{"name": "Ops prod", "config": map[string]any{"webhook_url": "********"}, "events": []string{notify.BackupFailure}}, nil), 200)
	expect(t, "test", alice.do("POST", base+"/notification-channels/"+created.ID+"/test", nil, nil), 200)
	if got := h.wait(t, 1); got[0]["type"] != notify.Test {
		t.Fatalf("test : %v", got[0])
	}

	// Diffusion via la file : seuls les événements abonnés partent, liens absolus.
	ctx := context.Background()
	e.jobs.Publish(ctx, notify.Event{Type: notify.BackupSuccess, TeamID: acme.ID, Title: "ok"})
	e.jobs.Publish(ctx, notify.Event{Type: notify.BackupFailure, TeamID: acme.ID, Title: "ko", URL: "/databases/x/backups"})
	e.jobs.Publish(ctx, notify.Event{Type: notify.BackupFailure, TeamID: "00000000-0000-0000-0000-000000000000", Title: "autre équipe"})
	got := h.wait(t, 2)
	time.Sleep(500 * time.Millisecond)
	if got = h.received(); len(got) != 2 || got[1]["title"] != "ko" || got[1]["url"] != "https://nodra.example.com/databases/x/backups" {
		t.Fatalf("diffusion : %v", got)
	}

	// Échec d'envoi : 502 sans le corps de la réponse, journalisé.
	h.setStatus(http.StatusInternalServerError)
	expect(t, "test en échec", alice.do("POST", base+"/notification-channels/"+created.ID+"/test", nil, nil), 502)
	var deliveries []store.NotificationDelivery
	expect(t, "journal", bob.do("GET", base+"/notification-deliveries", nil, &deliveries), 200)
	if len(deliveries) != 3 || deliveries[0].Status != "failed" || deliveries[1].Status != "sent" {
		t.Fatalf("journal : %+v", deliveries)
	}

	// Canal e-mail : nécessite l'envoi configuré sur l'instance.
	var mail struct {
		ID string `json:"id"`
	}
	expect(t, "destinataire invalide", alice.do("POST", base+"/notification-channels",
		map[string]any{"type": "email", "name": "Mail", "config": map[string]any{"recipients": []string{"a\r\nb@x"}}}, nil), 422)
	expect(t, "canal e-mail", alice.do("POST", base+"/notification-channels",
		map[string]any{"type": "email", "name": "Mail", "config": map[string]any{"recipients": []string{"ops@example.com"}}}, &mail), 201)
	expect(t, "e-mail non configuré", alice.do("POST", base+"/notification-channels/"+mail.ID+"/test", nil, nil), 502)
	resend := newHook(t)
	e.notifier.ResendAPI = resend.srv.URL
	expect(t, "config e-mail", alice.do("PUT", "/admin/mail", map[string]any{"provider": "resend", "resend_api_key": "re_123", "from_address": "nodra@example.com"}, nil), 200)
	expect(t, "test e-mail", alice.do("POST", base+"/notification-channels/"+mail.ID+"/test", nil, nil), 200)
	if r := resend.wait(t, 1); r[0]["subject"] != "[Nodra] Notification de test" || resend.auth[0] != "Bearer re_123" {
		t.Fatalf("resend : %v %v", r, resend.auth)
	}

	expect(t, "suppression par un lecteur", bob.do("DELETE", base+"/notification-channels/"+created.ID, nil, nil), 403)
	expect(t, "suppression", alice.do("DELETE", base+"/notification-channels/"+created.ID, nil, nil), 204)
	var bobco store.Team
	expect(t, "Bobco", bob.do("POST", "/teams", map[string]string{"name": "Bobco"}, &bobco), 201)
	expect(t, "canal d'une autre équipe", bob.do("DELETE", "/teams/"+bobco.ID+"/notification-channels/"+mail.ID, nil, nil), 404)
}

func TestInstanceMail(t *testing.T) {
	e := newTestEnv(t)
	alice, _ := register(t, e, "alice@example.com")
	bob, _ := register(t, e, "bob@example.com")
	if e.mailer.lastVerify != "" {
		t.Fatal("pas de vérification sans e-mail configuré")
	}
	expect(t, "non-admin", bob.do("GET", "/admin/mail", nil, nil), 403)
	var cfg struct {
		Configured bool              `json:"configured"`
		Config     notify.MailConfig `json:"config"`
	}
	expect(t, "lecture", alice.do("GET", "/admin/mail", nil, &cfg), 200)
	if cfg.Configured {
		t.Fatal("non configuré attendu")
	}
	expect(t, "provider inconnu", alice.do("PUT", "/admin/mail", map[string]any{"provider": "pigeon", "from_address": "a@b.c"}, nil), 422)
	expect(t, "expéditeur invalide", alice.do("PUT", "/admin/mail", map[string]any{"provider": "resend", "resend_api_key": "k", "from_address": "x"}, nil), 422)
	expect(t, "smtp", alice.do("PUT", "/admin/mail", map[string]any{"provider": "smtp", "host": "smtp.example.com", "port": 587,
		"encryption": "starttls", "username": "u", "password": "p4ss", "from_address": "nodra@example.com"}, &cfg), 200)
	if cfg.Config.Password != "********" {
		t.Fatalf("mot de passe masqué : %+v", cfg.Config)
	}
	expect(t, "mise à jour sans secret", alice.do("PUT", "/admin/mail", map[string]any{"provider": "smtp", "host": "smtp2.example.com", "port": 465,
		"encryption": "tls", "username": "u", "password": "********", "from_address": "nodra@example.com"}, nil), 200)
	m, err := e.notifier.MailConfig(context.Background())
	if err != nil || m.Password != "p4ss" || m.Host != "smtp2.example.com" {
		t.Fatalf("secret conservé : %+v %v", m, err)
	}

	// Jeton API d'un administrateur : refusé (session interactive requise).
	var tok struct {
		Token string `json:"token"`
	}
	expect(t, "jeton", alice.do("POST", "/me/tokens", map[string]any{"name": "ci", "scopes": []string{"root"}}, &tok), 201)
	api := newClient(t, e.srv)
	api.bearer = tok.Token
	expect(t, "admin via jeton", api.do("GET", "/admin/mail", nil, nil), 403)

	resend := newHook(t)
	e.notifier.ResendAPI = resend.srv.URL
	expect(t, "resend", alice.do("PUT", "/admin/mail", map[string]any{"provider": "resend", "resend_api_key": "re_1", "from_address": "nodra@example.com", "from_name": "Nodra"}, nil), 200)
	expect(t, "e-mail de test", alice.do("POST", "/admin/mail/test", map[string]string{}, nil), 200)
	if r := resend.wait(t, 1); r[0]["from"] != "Nodra <nodra@example.com>" || toJSON(r[0]["to"]) != `["alice@example.com"]` {
		t.Fatalf("resend : %v", r)
	}
	resend.setStatus(http.StatusUnauthorized)
	expect(t, "échec d'envoi", alice.do("POST", "/admin/mail/test", map[string]string{"to": "x@example.com"}, nil), 502)

	// Vérification d'adresse à l'inscription (e-mail configuré).
	carol, _ := register(t, e, "carol@example.com")
	token := e.mailer.lastVerify
	if token == "" {
		t.Fatal("e-mail de vérification attendu")
	}
	expect(t, "renvoi", carol.do("POST", "/me/verify-email", nil, nil), 204)
	if e.mailer.lastVerify == token {
		t.Fatal("nouveau jeton attendu")
	}
	expect(t, "jeton invalide", newClient(t, e.srv).do("POST", "/auth/verify-email", map[string]string{"token": "ndv_nope"}, nil), 400)
	expect(t, "vérification", newClient(t, e.srv).do("POST", "/auth/verify-email", map[string]string{"token": e.mailer.lastVerify}, nil), 204)
	expect(t, "jeton rejoué", newClient(t, e.srv).do("POST", "/auth/verify-email", map[string]string{"token": e.mailer.lastVerify}, nil), 400)
	var me struct{ User store.User }
	expect(t, "me", carol.do("GET", "/me", nil, &me), 200)
	if me.User.EmailVerifiedAt == nil {
		t.Fatal("adresse vérifiée attendue")
	}
	expect(t, "déjà vérifiée", carol.do("POST", "/me/verify-email", nil, nil), 409)

	expect(t, "suppression", alice.do("DELETE", "/admin/mail", nil, nil), 204)
	expect(t, "lecture", alice.do("GET", "/admin/mail", nil, &cfg), 200)
	if cfg.Configured {
		t.Fatal("configuration supprimée")
	}
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
