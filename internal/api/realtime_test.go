package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// wsConn : client WebSocket de test (cookies du client HTTP).
type wsConn struct {
	t *testing.T
	c *websocket.Conn
}

func (c *client) ws(path string, header http.Header) (*wsConn, int) {
	c.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, c.base+path, &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: header})
	if err != nil {
		if resp != nil {
			return nil, resp.StatusCode
		}
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() { _ = conn.CloseNow() })
	return &wsConn{c.t, conn}, http.StatusSwitchingProtocols
}

// next lit le prochain message : JSON décodé (texte) ou octets bruts (binaire, clé "binary").
func (w *wsConn) next() map[string]any {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	typ, data, err := w.c.Read(ctx)
	if err != nil {
		w.t.Fatalf("lecture WebSocket : %v", err)
	}
	if typ == websocket.MessageBinary {
		return map[string]any{"type": "binary", "data": string(data)}
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		w.t.Fatal(err)
	}
	return m
}

// until lit jusqu'au message de type want et renvoie tous les messages lus.
func (w *wsConn) until(want string) []map[string]any {
	w.t.Helper()
	var all []map[string]any
	for {
		m := w.next()
		all = append(all, m)
		if m["type"] == want {
			return all
		}
		if m["type"] == "error" && want != "error" {
			w.t.Fatalf("erreur : %v", m["message"])
		}
	}
}

func (w *wsConn) send(v any) {
	b, _ := json.Marshal(v)
	if err := w.c.Write(context.Background(), websocket.MessageText, b); err != nil {
		w.t.Fatal(err)
	}
}

func binaryText(msgs []map[string]any) string {
	var b strings.Builder
	for _, m := range msgs {
		if m["type"] == "binary" || m["type"] == "output" {
			b.WriteString(m["data"].(string))
		}
	}
	return b.String()
}

func TestRealtime(t *testing.T) {
	f := newAppFixture(t)
	h := newFakeHost(t, f)
	alice, base := f.alice, f.base
	<-f.e.hub.Ready()
	bob, bobID := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)

	var app map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(nil), &app), 201)
	appID := app["id"].(string)

	// Origine étrangère et absence d'authentification refusées.
	if _, code := alice.ws(base+"/events", http.Header{"Origin": {"https://evil.example"}}); code != http.StatusForbidden {
		t.Fatalf("origine étrangère : %d", code)
	}
	if _, code := newClient(t, f.e.srv).ws(base+"/events", nil); code != http.StatusUnauthorized {
		t.Fatalf("anonyme : %d", code)
	}

	// Bob ne peut pas lire l'application : ses événements ne lui parviennent pas.
	expect(t, "refus sur l'application", alice.do("POST", base+"/role-assignments", map[string]any{"user_id": bobID,
		"role_id": roleID(t, alice, f.team, rbac.RoleViewer), "scope": map[string]string{"type": "resource", "id": appID}, "effect": "deny"}, nil), 201)
	events, _ := alice.ws(base+"/events", nil)
	bobEvents, _ := bob.ws(base+"/events", nil)
	if m := events.next(); m["type"] != "ready" {
		t.Fatalf("ready : %v", m)
	}
	bobEvents.next()

	// Déploiement lent suivi en direct.
	f.e.api.Executors["resource.deploy"] = f.e.api.execResourceDeploy
	h.mu.Lock()
	h.env["SLOW"] = "1"
	h.mu.Unlock()
	var d store.Deployment
	expect(t, "déploiement", alice.do("POST", base+"/applications/"+appID+"/deploy", nil, &d), 202)
	logs, _ := alice.ws(base+"/applications/"+appID+"/deployments/"+d.ID+"/logs/stream", nil)
	msgs := logs.until("end")
	var lines int
	for _, m := range msgs {
		if m["type"] == "logs" {
			lines += len(m["logs"].([]any))
		}
	}
	if end := msgs[len(msgs)-1]; end["status"] != "finished" || lines == 0 {
		t.Fatalf("flux du déploiement : %v", msgs)
	}
	if depLogs(t, alice, base, appID, d.ID, false) == "" {
		t.Fatal("logs attendus")
	}
	// Événements : statut du déploiement reçu par Alice, pas par Bob.
	deadline := time.Now().Add(10 * time.Second)
	for {
		m := events.next()
		e, _ := m["event"].(map[string]any)
		if e["kind"] == store.EventDeploymentStatus && e["status"] == "finished" && e["resource"] == appID {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("événement de fin de déploiement non reçu")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	if _, data, err := bobEvents.c.Read(ctx); err == nil {
		t.Fatalf("Bob ne doit rien recevoir : %s", data)
	}
	cancel()

	// Conteneurs, logs suivis, terminal du conteneur.
	var containers []map[string]any
	expect(t, "conteneurs", alice.do("GET", base+"/applications/"+appID+"/containers", nil, &containers), 200)
	if len(containers) != 1 || !strings.HasPrefix(containers[0]["name"].(string), appID) {
		t.Fatalf("conteneurs : %v", containers)
	}
	cont := containers[0]["name"].(string)
	follow, _ := alice.ws(base+"/applications/"+appID+"/logs/stream?tail=10", nil)
	if out := binaryText(follow.until("end")); !strings.Contains(out, "serveur démarré") {
		t.Fatalf("logs suivis : %q", out)
	}
	bad, _ := alice.ws(base+"/applications/"+appID+"/logs/stream?container=autre", nil)
	if m := bad.until("error"); !strings.Contains(m[len(m)-1]["message"].(string), "introuvable") {
		t.Fatalf("conteneur inconnu : %v", m)
	}
	if _, code := bob.ws(base+"/applications/"+appID+"/terminal", nil); code != http.StatusForbidden {
		t.Fatalf("terminal sans droit : %d", code)
	}
	term, _ := alice.ws(base+"/applications/"+appID+"/terminal?rows=30&cols=90", nil)
	if m := term.next(); m["type"] != "ready" || m["container"] != cont {
		t.Fatalf("terminal : %v", m)
	}
	if out := binaryText(term.until("exit")); !strings.Contains(out, "exécuté : -it -e TERM=xterm-256color "+cont) {
		t.Fatalf("terminal du conteneur : %q", out)
	}

	// Terminal du serveur : shell interactif, redimensionnement, audit.
	sterm, _ := alice.ws(base+"/servers/"+f.serverID+"/terminal", nil)
	if m := sterm.next(); m["type"] != "ready" {
		t.Fatalf("terminal serveur : %v", m)
	}
	sterm.send(map[string]any{"type": "resize", "rows": 50, "cols": 160})
	sterm.send(map[string]any{"type": "input", "data": "echo bonjour\n"})
	if err := sterm.c.Write(context.Background(), websocket.MessageBinary, []byte("exit 3\n")); err != nil {
		t.Fatal(err)
	}
	msgs = sterm.until("exit")
	if out := binaryText(msgs); !strings.Contains(out, "bonjour") || msgs[len(msgs)-1]["code"] != float64(3) {
		t.Fatalf("terminal serveur : %v", msgs)
	}
	var resized bool
	for _, p := range f.ssh.Ptys() {
		if p.Command == "" && len(p.Sizes) == 2 && p.Sizes[1] == [2]int{50, 160} {
			resized = true
		}
	}
	if !resized {
		t.Fatalf("redimensionnement non transmis : %+v", f.ssh.Ptys())
	}
	var audit struct{ Events []store.AuditEvent }
	expect(t, "audit", alice.do("GET", base+"/audit?action=server.terminal.opened", nil, &audit), 200)
	if len(audit.Events) != 1 {
		t.Fatalf("audit du terminal : %+v", audit)
	}

	// Sortie d'une tâche serveur en direct.
	var task store.ServerTask
	expect(t, "commande", alice.do("POST", base+"/servers/"+f.serverID+"/commands", map[string]string{"command": "echo ligne-1; echo ligne-2"}, &task), 202)
	ts, _ := bob.ws(base+"/servers/"+f.serverID+"/tasks/"+task.ID+"/stream", nil)
	msgs = ts.until("end")
	if out := binaryText(msgs); !strings.Contains(out, "ligne-2") || msgs[len(msgs)-1]["status"] != "success" {
		t.Fatalf("tâche : %v", msgs)
	}

	// Logs du proxy suivis.
	px, _ := bob.ws(base+"/servers/"+f.serverID+"/proxy/logs/stream", nil)
	if out := binaryText(px.until("end")); !strings.Contains(out, "serveur démarré") {
		t.Fatalf("logs du proxy : %q", out)
	}
}
