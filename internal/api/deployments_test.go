package api

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// fakeHost exécute réellement les scripts de déploiement (sh -s) avec de faux docker/git/nixpacks.
type fakeHost struct {
	state string
	mu    sync.Mutex
	env   map[string]string
}

func (h *fakeHost) set(k, v string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.env[k] = v
}

func (h *fakeHost) calls() string {
	b, _ := os.ReadFile(filepath.Join(h.state, "calls"))
	return string(b)
}

func (h *fakeHost) containers() []string {
	entries, _ := os.ReadDir(filepath.Join(h.state, "containers"))
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func newFakeHost(t *testing.T, f *appFixture) *fakeHost {
	bin, _ := filepath.Abs("testdata/fakebin")
	h := &fakeHost{state: t.TempDir(), env: map[string]string{}}
	f.e.runner.AppsDir = filepath.Join(h.state, "apps")
	var dockerOK atomic.Bool
	dockerOK.Store(true)
	fallback := fakeUbuntu(&dockerOK)
	f.ssh.SetInputHandler(func(cmd string, stdin io.Reader, stdout, stderr io.Writer) int {
		// Lecture/écriture de fichiers (téléchargement, import) et commandes des terminaux et logs suivis :
		// exécutées réellement, comme les scripts ; un shell interactif (commande vide) lit ses commandes sur stdin.
		fileCmd := strings.HasPrefix(cmd, "cat ") || strings.HasPrefix(cmd, "mkdir -p ") || strings.HasPrefix(cmd, "rm -f ") ||
			strings.HasPrefix(cmd, "sh -c ")
		if cmd != "sh -s" && cmd != "" && !fileCmd {
			return fallback(cmd, stdout, stderr)
		}
		c := exec.Command("sh", "-s")
		if fileCmd {
			c = exec.Command("sh", "-c", cmd)
		}
		c.Stdout, c.Stderr = stdout, stderr
		c.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "STATE="+h.state)
		h.mu.Lock()
		for k, v := range h.env {
			c.Env = append(c.Env, k+"="+v)
		}
		h.mu.Unlock()
		// Entrée copiée à part : la fin de la commande ne dépend pas de la fermeture de l'entrée par le client
		// (terminal, logs suivis), comme avec un vrai sshd.
		in, _ := c.StdinPipe()
		go func() {
			_, _ = io.Copy(in, stdin)
			in.Close()
		}()
		if err := c.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			return 127
		}
		return 0
	})
	return h
}

func waitDeployment(t *testing.T, c *client, base, appID, depID string) store.Deployment {
	t.Helper()
	var d store.Deployment
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		expect(t, "déploiement", c.do("GET", base+"/applications/"+appID+"/deployments/"+depID, nil, &d), 200)
		if d.Status != "queued" && d.Status != "in_progress" {
			return d
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("déploiement %s non terminé (%s)", depID, d.Status)
	return d
}

func depLogs(t *testing.T, c *client, base, appID, depID string, debug bool) string {
	t.Helper()
	var out struct {
		Logs []store.DeploymentLog `json:"logs"`
	}
	q := ""
	if debug {
		q = "?debug=1"
	}
	expect(t, "logs", c.do("GET", base+"/applications/"+appID+"/deployments/"+depID+"/logs"+q, nil, &out), 200)
	var b strings.Builder
	for _, l := range out.Logs {
		b.WriteString(l.Message + "\n")
	}
	return b.String()
}

func TestDeployments(t *testing.T) {
	f := newAppFixture(t)
	h := newFakeHost(t, f)
	alice, base := f.alice, f.base
	f.e.api.Executors["resource.deploy"] = f.e.api.execDeploy // le vrai exécuteur (l'environnement de test en installe un faux)

	var app map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Site"}), &app), 201)
	appID := app["id"].(string)
	expect(t, "secret", alice.do("POST", base+"/applications/"+appID+"/envs", map[string]any{"key": "API_KEY", "value": "valeur-tres-secrete", "is_secret": true}, nil), 200)
	expect(t, "fichier", alice.do("POST", base+"/applications/"+appID+"/storages", map[string]any{"kind": "file", "mount_path": "/app/config.json", "content": `{"ok":true}`}, nil), 201)

	deploy := func(body map[string]any) store.Deployment {
		t.Helper()
		var d store.Deployment
		expect(t, "déployer", alice.do("POST", base+"/applications/"+appID+"/deploy", body, &d), 202)
		return waitDeployment(t, alice, base, appID, d.ID)
	}

	// Premier déploiement : clone, nixpacks, build, démarrage.
	d1 := deploy(nil)
	logs := depLogs(t, alice, base, appID, d1.ID, true)
	if d1.Status != "finished" {
		t.Fatalf("déploiement : %s %s\n%s", d1.Status, d1.Error, logs)
	}
	if !strings.HasPrefix(d1.Image, "nodra/"+appID+":0123456789ab-") || d1.CommitSHA != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("image/commit : %s %s", d1.Image, d1.CommitSHA)
	}
	for _, want := range []string{"git clone", "nixpacks build", "docker compose up -d", "Conteneur sain", "Déploiement terminé"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log manquant : %q\n%s", want, logs)
		}
	}
	if strings.Contains(logs, "valeur-tres-secrete") {
		t.Fatal("la valeur secrète ne doit jamais apparaître dans les logs")
	}
	calls := h.calls()
	if !strings.Contains(calls, "nixpacks build . --out .") || !strings.Contains(calls, "-f .nixpacks/Dockerfile -t "+d1.Image) ||
		!strings.Contains(calls, "--label nodra.resource="+appID) {
		t.Fatalf("appels :\n%s", calls)
	}
	if cs := h.containers(); len(cs) != 1 {
		t.Fatalf("1 conteneur attendu : %v", cs)
	}
	if b, err := os.ReadFile(filepath.Join(h.state, "apps", appID, "files")); err == nil && len(b) > 0 {
		t.Fatal("files doit être un dossier")
	}
	var got map[string]any
	expect(t, "statut", alice.do("GET", base+"/applications/"+appID, nil, &got), 200)
	if got["application"].(map[string]any)["status"] != "running" {
		t.Fatal("l'application doit être en cours d'exécution")
	}

	// Redéploiement sans changement : image réutilisée, bascule sans coupure.
	first := h.containers()[0]
	d2 := deploy(nil)
	logs = depLogs(t, alice, base, appID, d2.ID, false)
	if d2.Status != "finished" || !strings.Contains(logs, "construction ignorée") || d2.Image != d1.Image {
		t.Fatalf("redéploiement : %s\n%s", d2.Status, logs)
	}
	if cs := h.containers(); len(cs) != 1 || cs[0] == first {
		t.Fatalf("l'ancien conteneur doit être remplacé : %v (avant %s)", cs, first)
	}

	// Reconstruction forcée avec un nouveau commit, puis retour arrière.
	h.set("SHA", "fedcba9876543210fedcba9876543210fedcba98")
	d3 := deploy(map[string]any{"force_rebuild": true})
	if d3.Status != "finished" || !strings.HasPrefix(d3.Image, "nodra/"+appID+":fedcba987654-") {
		t.Fatalf("nouveau commit : %s %s", d3.Status, d3.Image)
	}
	var images []struct {
		Tag     string
		Current bool
	}
	expect(t, "images", alice.do("GET", base+"/applications/"+appID+"/images", nil, &images), 200)
	if len(images) != 2 {
		t.Fatalf("2 images attendues : %+v", images)
	}
	oldTag := strings.TrimPrefix(d1.Image, "nodra/"+appID+":")
	var rb store.Deployment
	expect(t, "tag invalide", alice.do("POST", base+"/applications/"+appID+"/rollback", map[string]string{"tag": "../x"}, nil), 422)
	expect(t, "retour arrière", alice.do("POST", base+"/applications/"+appID+"/rollback", map[string]string{"tag": oldTag}, &rb), 202)
	rb = waitDeployment(t, alice, base, appID, rb.ID)
	if rb.Status != "finished" || rb.Image != d1.Image || rb.Trigger != "rollback" {
		t.Fatalf("retour arrière : %+v", rb)
	}

	// Échec de build : l'ancien conteneur reste en place.
	before := h.containers()
	h.set("FAIL_BUILD", "1")
	h.set("SHA", "1111111111111111111111111111111111111111")
	d4 := deploy(nil)
	logs = depLogs(t, alice, base, appID, d4.ID, false)
	if d4.Status != "failed" || !strings.Contains(logs, "erreur de compilation") {
		t.Fatalf("échec de build attendu : %s\n%s", d4.Status, logs)
	}
	if cs := h.containers(); len(cs) != 1 || cs[0] != before[0] {
		t.Fatalf("l'ancien conteneur doit être conservé : %v", cs)
	}
	h.set("FAIL_BUILD", "")

	// Nouveau conteneur en mauvaise santé : supprimé, l'ancien est conservé.
	h.set("HEALTH", "unhealthy")
	d5 := deploy(map[string]any{"force_rebuild": true})
	if d5.Status != "failed" || !strings.Contains(d5.Error, "mauvaise santé") {
		t.Fatalf("échec de santé attendu : %s %s", d5.Status, d5.Error)
	}
	if cs := h.containers(); len(cs) != 1 || cs[0] != before[0] {
		t.Fatalf("seul l'ancien conteneur doit rester : %v", cs)
	}
	h.set("HEALTH", "")

	// Annulation d'un build long.
	h.set("SLOW", "20")
	var d6 store.Deployment
	expect(t, "déploiement lent", alice.do("POST", base+"/applications/"+appID+"/deploy", map[string]any{"force_rebuild": true}, &d6), 202)
	for i := 0; i < 100; i++ {
		var cur store.Deployment
		alice.do("GET", base+"/applications/"+appID+"/deployments/"+d6.ID, nil, &cur)
		if cur.Status == "in_progress" && strings.Contains(depLogs(t, alice, base, appID, d6.ID, false), "nixpacks") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	expect(t, "annulation", alice.do("POST", base+"/applications/"+appID+"/deployments/"+d6.ID+"/cancel", nil, nil), 204)
	start := time.Now()
	if d6 = waitDeployment(t, alice, base, appID, d6.ID); d6.Status != "cancelled" || time.Since(start) > 15*time.Second {
		t.Fatalf("annulation : %s (%s)", d6.Status, time.Since(start))
	}
	h.set("SLOW", "")
	expect(t, "annulation d'un déploiement terminé", alice.do("POST", base+"/applications/"+appID+"/deployments/"+d6.ID+"/cancel", nil, nil), 409)

	// Déploiement soumis à approbation.
	dave, _ := register(t, f.e, "dave@example.com")
	invite(t, f.e, alice, dave, f.team, "dave@example.com", rbac.RoleAdmin)
	expect(t, "règle", alice.do("POST", base+"/approval-rules", map[string]any{"name": "Prod", "actions": []string{"resource.deploy"}}, nil), 201)
	var pending struct {
		ApprovalRequest store.ApprovalRequest `json:"approval_request"`
	}
	expect(t, "déploiement soumis", alice.do("POST", base+"/applications/"+appID+"/deploy", nil, &pending), 202)
	var appr store.ApprovalRequest
	expect(t, "approbation", dave.do("POST", base+"/approvals/"+pending.ApprovalRequest.ID+"/approve", nil, &appr), 200)
	if appr.Status != "executed" {
		t.Fatalf("approbation : %s %s", appr.Status, appr.Error)
	}
	var deps []store.Deployment
	expect(t, "historique", alice.do("GET", base+"/applications/"+appID+"/deployments", nil, &deps), 200)
	if deps[0].Trigger != "approval" {
		t.Fatalf("déploiement créé par l'approbation : %+v", deps[0])
	}
	waitDeployment(t, alice, base, appID, deps[0].ID)
	expect(t, "suppression de la règle", alice.do("DELETE", base+"/approval-rules/"+*pending.ApprovalRequest.RuleID, nil, nil), 204)

	// Logs des conteneurs, arrêt, redémarrage.
	var cl struct{ Logs string }
	expect(t, "logs conteneurs", alice.do("GET", base+"/applications/"+appID+"/logs?lines=50", nil, &cl), 200)
	if !strings.Contains(cl.Logs, "serveur démarré") {
		t.Fatalf("logs : %q", cl.Logs)
	}
	expect(t, "arrêt", alice.do("POST", base+"/applications/"+appID+"/stop", nil, nil), 204)
	if cs := h.containers(); len(cs) != 0 {
		t.Fatalf("aucun conteneur après arrêt : %v", cs)
	}
	var restart store.Deployment
	expect(t, "redémarrage", alice.do("POST", base+"/applications/"+appID+"/restart", nil, &restart), 202)
	if restart = waitDeployment(t, alice, base, appID, restart.ID); restart.Status != "finished" || restart.RollbackImage == "" {
		t.Fatalf("redémarrage sans reconstruction : %+v", restart)
	}

	// Image Docker et Docker Compose.
	var img, stack map[string]any
	expect(t, "image", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Img", "build_pack": "dockerimage", "git_repository": "", "docker_image": "nginx"}), &img), 201)
	var di store.Deployment
	expect(t, "déploiement image", alice.do("POST", base+"/applications/"+img["id"].(string)+"/deploy", nil, &di), 202)
	if di = waitDeployment(t, alice, base, img["id"].(string), di.ID); di.Status != "finished" || di.Image != "nginx:latest" {
		t.Fatalf("image Docker : %+v", di)
	}
	expect(t, "compose", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Stack", "build_pack": "dockercompose", "git_repository": "",
		"docker_compose_raw": "services:\n  web:\n    image: nginx\n    volumes: [\"./html:/usr/share/nginx/html\"]\n"}), &stack), 201)
	stackID := stack["id"].(string)
	expect(t, "variable compose", alice.do("POST", base+"/applications/"+stackID+"/envs", map[string]any{"key": "PRICE", "value": "10$", "is_literal": true}, nil), 200)
	var dc store.Deployment
	expect(t, "déploiement compose", alice.do("POST", base+"/applications/"+stackID+"/deploy", nil, &dc), 202)
	if dc = waitDeployment(t, alice, base, stackID, dc.ID); dc.Status != "finished" {
		t.Fatalf("compose : %s %s\n%s", dc.Status, dc.Error, depLogs(t, alice, base, stackID, dc.ID, true))
	}
	envFile, _ := os.ReadFile(filepath.Join(h.state, "apps", stackID, ".env"))
	transformed, _ := os.ReadFile(filepath.Join(h.state, "apps", stackID, "docker-compose.yml"))
	if !strings.Contains(string(envFile), "PRICE='10$'") || !strings.Contains(string(transformed), filepath.Join(h.state, "apps", stackID, "data")+"/html:/usr/share/nginx/html") {
		t.Fatalf(".env / compose transformé :\n%s\n%s", envFile, transformed)
	}

	// Suppression : conteneurs et fichiers nettoyés.
	expect(t, "suppression", alice.do("DELETE", base+"/applications/"+appID, nil, nil), 204)
	if _, err := os.Stat(filepath.Join(h.state, "apps", appID)); !os.IsNotExist(err) {
		t.Fatal("le dossier de l'application doit être supprimé")
	}
}
