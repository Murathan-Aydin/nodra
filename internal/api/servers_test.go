package api

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/ssh/sshtest"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// fakeUbuntu simule un serveur Ubuntu avec Docker ; dockerOK permet de simuler Docker absent.
func fakeUbuntu(dockerOK *atomic.Bool) sshtest.Handler {
	return func(cmd string, stdout, stderr io.Writer) int {
		switch {
		case cmd == "uptime":
			fmt.Fprintln(stdout, " 12:00:00 up 3 days,  load average: 0.10, 0.20, 0.30")
		case cmd == "cat /etc/os-release":
			fmt.Fprintln(stdout, "NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"24.04\"")
		case strings.HasPrefix(cmd, "docker version"):
			if !dockerOK.Load() {
				fmt.Fprintln(stderr, "docker: command not found")
				return 127
			}
			fmt.Fprintln(stdout, "27.3.1")
		case cmd == "docker compose version --short":
			fmt.Fprintln(stdout, "2.29.7")
		case strings.HasPrefix(cmd, "head -1 /proc/stat"):
			fmt.Fprint(stdout, "cpu  100 0 100 800 0 0 0 0\ncpu  150 0 150 850 0 0 0 0\nMemTotal: 2000000 kB\nMemAvailable: 500000 kB\n/dev/sda1 10000000 2500000 7500000 25% /\n0.42 0.30 0.20 1/100 1\n")
		case strings.HasPrefix(cmd, "echo "):
			fmt.Fprintln(stdout, strings.TrimPrefix(cmd, "echo "))
		}
		return 0
	}
}

func waitTask(t *testing.T, c *client, teamID, serverID, taskID string) store.ServerTask {
	t.Helper()
	var task store.ServerTask
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		expect(t, "tâche", c.do("GET", "/teams/"+teamID+"/servers/"+serverID+"/tasks/"+taskID, nil, &task), 200)
		if task.Status != "queued" && task.Status != "running" {
			return task
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("tâche %s non terminée (statut %s)", taskID, task.Status)
	return task
}

func TestServers(t *testing.T) {
	e := newTestEnv(t)
	alice, _ := register(t, e, "alice@example.com")
	bob, _ := register(t, e, "bob@example.com")
	var acme store.Team
	expect(t, "Acme", alice.do("POST", "/teams", map[string]string{"name": "Acme"}, &acme), 201)
	invite(t, e, alice, bob, acme.ID, "bob@example.com", rbac.RoleDeveloper)
	base := "/teams/" + acme.ID

	// Clés : génération, import invalide, aucune fuite de la clé privée.
	var key store.PrivateKey
	expect(t, "clé générée", alice.do("POST", base+"/private-keys", map[string]string{"name": "prod"}, &key), 201)
	if !strings.HasPrefix(key.PublicKey, "ssh-ed25519 ") || !strings.HasPrefix(key.Fingerprint, "SHA256:") {
		t.Fatalf("clé publique inattendue : %+v", key)
	}
	expect(t, "clé invalide", alice.do("POST", base+"/private-keys", map[string]string{"name": "x", "private_key": "nope"}, nil), 422)
	priv, _, _ := ssh.GenerateKey("import")
	expect(t, "clé importée", alice.do("POST", base+"/private-keys", map[string]string{"name": "importée", "private_key": priv}, nil), 201)
	expect(t, "développeur sans key.manage", bob.do("POST", base+"/private-keys", map[string]string{"name": "b"}, nil), 403)

	var dockerOK atomic.Bool
	dockerOK.Store(true)
	srv := sshtest.New(t, key.PublicKey, fakeUbuntu(&dockerOK))

	// Création : validation automatique.
	var created struct {
		Server store.Server     `json:"server"`
		Task   store.ServerTask `json:"task"`
	}
	expect(t, "IP invalide", alice.do("POST", base+"/servers", map[string]any{"name": "s", "ip": "pas une ip!", "private_key_id": key.ID}, nil), 422)
	expect(t, "création par développeur", bob.do("POST", base+"/servers", map[string]any{"name": "s", "ip": "127.0.0.1", "private_key_id": key.ID}, nil), 403)
	expect(t, "création", alice.do("POST", base+"/servers", map[string]any{"name": "prod-1", "ip": srv.Host, "port": srv.Port,
		"private_key_id": key.ID}, &created), 201)
	sid := created.Server.ID
	task := waitTask(t, alice, acme.ID, sid, created.Task.ID)
	if task.Status != "success" || !strings.Contains(task.Output, "Serveur prêt") {
		t.Fatalf("validation : %s %s\n%s", task.Status, task.Error, task.Output)
	}
	var detail struct {
		Server       store.Server         `json:"server"`
		Destinations []*store.Destination `json:"destinations"`
	}
	expect(t, "détail serveur", bob.do("GET", base+"/servers/"+sid, nil, &detail), 200)
	sv := detail.Server
	if !sv.IsUsable || sv.DockerVersion != "27.3.1" || sv.OSID != "ubuntu" || sv.HostKey != srv.AuthorizedHostKey() {
		t.Fatalf("serveur après validation : %+v", sv)
	}
	if len(detail.Destinations) != 1 || detail.Destinations[0].Network != "nodra" || !detail.Destinations[0].IsDefault {
		t.Fatalf("destination par défaut : %+v", detail.Destinations)
	}
	expect(t, "clé utilisée", alice.do("DELETE", base+"/private-keys/"+key.ID, nil, nil), 409)

	// Commandes : permission server.terminal, sortie enregistrée, audit.
	expect(t, "commande par développeur", bob.do("POST", base+"/servers/"+sid+"/commands", map[string]string{"command": "echo hi"}, nil), 403)
	var tk store.ServerTask
	expect(t, "commande", alice.do("POST", base+"/servers/"+sid+"/commands", map[string]string{"command": "echo bonjour"}, &tk), 202)
	tk = waitTask(t, alice, acme.ID, sid, tk.ID)
	if tk.Status != "success" || !strings.Contains(tk.Output, "$ echo bonjour\nbonjour") {
		t.Fatalf("commande : %s\n%s", tk.Status, tk.Output)
	}

	// Cloudflare Tunnel : le jeton n'apparaît ni dans la sortie ni dans les paramètres.
	token := "eyJhIjoiMTIzNDU2Nzg5MCIsInQiOiJzZWNyZXQifQ"
	expect(t, "cloudflared", alice.do("PUT", base+"/servers/"+sid+"/cloudflared", map[string]string{"token": token}, &tk), 202)
	tk = waitTask(t, alice, acme.ID, sid, tk.ID)
	if tk.Status != "success" || strings.Contains(tk.Output, token) || strings.Contains(string(tk.Params), token) {
		t.Fatalf("le jeton Cloudflare ne doit pas fuiter : %s\n%s\n%s", tk.Status, tk.Output, tk.Params)
	}
	if !slices.ContainsFunc(srv.Received(), func(c string) bool { return strings.Contains(c, token) }) {
		t.Fatal("le jeton doit être transmis au serveur")
	}

	// Nettoyage Docker
	expect(t, "nettoyage", alice.do("POST", base+"/servers/"+sid+"/docker-cleanup", nil, &tk), 202)
	if tk = waitTask(t, alice, acme.ID, sid, tk.ID); tk.Status != "success" || !strings.Contains(tk.Output, "docker container prune") {
		t.Fatalf("nettoyage : %s\n%s", tk.Status, tk.Output)
	}

	// Destinations
	expect(t, "réseau invalide", alice.do("POST", base+"/servers/"+sid+"/destinations", map[string]string{"network": "a b"}, nil), 422)
	expect(t, "réseau web", alice.do("POST", base+"/servers/"+sid+"/destinations", map[string]string{"name": "Web", "network": "web"}, &tk), 202)
	waitTask(t, alice, acme.ID, sid, tk.ID)
	var dests []*store.Destination
	expect(t, "destinations", alice.do("GET", base+"/servers/"+sid+"/destinations", nil, &dests), 200)
	if len(dests) != 2 {
		t.Fatalf("2 destinations attendues : %+v", dests)
	}
	for _, d := range dests {
		if d.IsDefault {
			expect(t, "suppression destination par défaut", alice.do("DELETE", base+"/servers/"+sid+"/destinations/"+d.ID, nil, nil), 409)
		} else {
			expect(t, "suppression web", alice.do("DELETE", base+"/servers/"+sid+"/destinations/"+d.ID, nil, &tk), 202)
			waitTask(t, alice, acme.ID, sid, tk.ID)
		}
	}
	expect(t, "destinations", alice.do("GET", base+"/servers/"+sid+"/destinations", nil, &dests), 200)
	if len(dests) != 1 {
		t.Fatal("la destination web doit être supprimée")
	}

	// Métriques
	if err := e.svc.CollectMetrics(context.Background()); err != nil {
		t.Fatal(err)
	}
	var metrics []store.ServerMetric
	expect(t, "métriques", alice.do("GET", base+"/servers/"+sid+"/metrics", nil, &metrics), 200)
	if len(metrics) != 1 || metrics[0].MemTotal != 2000000*1024 {
		t.Fatalf("métriques : %+v", metrics)
	}

	// Validation en échec : Docker absent.
	dockerOK.Store(false)
	expect(t, "revalidation", alice.do("POST", base+"/servers/"+sid+"/validate", nil, &tk), 202)
	if tk = waitTask(t, alice, acme.ID, sid, tk.ID); tk.Status != "failed" {
		t.Fatalf("validation sans Docker doit échouer : %s", tk.Status)
	}
	expect(t, "détail", alice.do("GET", base+"/servers/"+sid, nil, &detail), 200)
	if detail.Server.IsUsable || !strings.Contains(detail.Server.ValidationError, "Docker") {
		t.Fatalf("serveur inutilisable attendu : %+v", detail.Server)
	}
	dockerOK.Store(true)
	expect(t, "installation Docker", alice.do("POST", base+"/servers/"+sid+"/install-docker", nil, &tk), 202)
	if tk = waitTask(t, alice, acme.ID, sid, tk.ID); tk.Status != "success" {
		t.Fatalf("installation : %s %s", tk.Status, tk.Error)
	}

	// Clé d'hôte modifiée : refus, puis réinitialisation.
	other := sshtest.New(t, key.PublicKey, fakeUbuntu(&dockerOK))
	if _, err := e.st.Pool.Exec(context.Background(), `UPDATE servers SET host_key = $1 WHERE id = $2`, other.AuthorizedHostKey(), sid); err != nil {
		t.Fatal(err)
	}
	expect(t, "commande clé d'hôte changée", alice.do("POST", base+"/servers/"+sid+"/commands", map[string]string{"command": "echo x"}, &tk), 202)
	if tk = waitTask(t, alice, acme.ID, sid, tk.ID); tk.Status != "failed" || !strings.Contains(tk.Error, "clé d'hôte") {
		t.Fatalf("la clé d'hôte différente doit bloquer : %s %s", tk.Status, tk.Error)
	}
	expect(t, "reset clé d'hôte", alice.do("POST", base+"/servers/"+sid+"/host-key/reset", nil, nil), 204)
	expect(t, "revalidation", alice.do("POST", base+"/servers/"+sid+"/validate", nil, &tk), 202)
	if tk = waitTask(t, alice, acme.ID, sid, tk.ID); tk.Status != "success" {
		t.Fatalf("revalidation après reset : %s %s", tk.Status, tk.Error)
	}

	// Réglages
	expect(t, "cron invalide", alice.do("PATCH", base+"/servers/"+sid, map[string]any{"settings": map[string]any{"docker_cleanup_frequency": "jamais"}}, nil), 422)
	var updated store.Server
	expect(t, "réglages", alice.do("PATCH", base+"/servers/"+sid, map[string]any{"settings": map[string]any{"docker_cleanup_frequency": "@weekly", "concurrent_builds": 4}}, &updated), 200)
	if updated.Settings.DockerCleanupFrequency != "@weekly" || updated.Settings.ConcurrentBuilds != 4 || updated.Settings.MetricsRetentionDays != 7 || updated.HostKey == "" {
		t.Fatalf("réglages fusionnés attendus : %+v", updated.Settings)
	}
	expect(t, "changement de port", alice.do("PATCH", base+"/servers/"+sid, map[string]any{"port": 2222}, &updated), 200)
	if updated.HostKey != "" || updated.IsUsable {
		t.Fatal("changer d'adresse invalide la clé d'hôte et la validation")
	}

	// Historique des tâches (sans sortie) et audit.
	var tasks []store.ServerTask
	expect(t, "tâches", bob.do("GET", base+"/servers/"+sid+"/tasks", nil, &tasks), 200)
	if len(tasks) < 8 || tasks[0].Output != "" {
		t.Fatalf("historique des tâches : %d", len(tasks))
	}

	// Suppression soumise à approbation, puis directe.
	expect(t, "règle", alice.do("POST", base+"/approval-rules", map[string]any{"name": "Serveurs", "actions": []string{"server.delete"}}, nil), 201)
	expect(t, "suppression soumise", alice.do("DELETE", base+"/servers/"+sid, nil, nil), 202)
	expect(t, "serveur toujours là", alice.do("GET", base+"/servers/"+sid, nil, nil), 200)
	var reqs []store.ApprovalRequest
	expect(t, "demandes", alice.do("GET", base+"/approvals?status=pending", nil, &reqs), 200)
	expect(t, "invitation dave", alice.do("POST", base+"/invitations", map[string]any{"email": "dave@example.com", "role_id": roleID(t, alice, acme.ID, rbac.RoleAdmin)}, nil), 201)
	dave, _ := register(t, e, "dave@example.com")
	expect(t, "dave accepte", dave.do("POST", "/invitations/accept", map[string]string{"token": e.mailer.lastInvite}, nil), 200)
	var got store.ApprovalRequest
	expect(t, "approbation", dave.do("POST", base+"/approvals/"+reqs[0].ID+"/approve", nil, &got), 200)
	if got.Status != "executed" {
		t.Fatalf("suppression approuvée : %s %s", got.Status, got.Error)
	}
	expect(t, "serveur supprimé", alice.do("GET", base+"/servers/"+sid, nil, nil), 404)
	expect(t, "clé libérée", alice.do("DELETE", base+"/private-keys/"+key.ID, nil, nil), 204)
}
