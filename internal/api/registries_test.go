package api

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestDockerRegistries(t *testing.T) {
	f := newAppFixture(t)
	h := newFakeHost(t, f)
	alice, base := f.alice, f.base
	bob, _ := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)
	f.e.api.Executors["resource.deploy"] = f.e.api.execResourceDeploy

	reg := map[string]string{"name": "GHCR", "host": "https://GHCR.io/", "username": "acme", "password": "ghp_secret"}
	expect(t, "lecteur", bob.do("POST", base+"/docker-registries", reg, nil), 403)
	expect(t, "hôte invalide", alice.do("POST", base+"/docker-registries", map[string]string{"name": "x", "host": "ghcr.io/acme", "username": "a", "password": "b"}, nil), 422)
	expect(t, "mot de passe requis", alice.do("POST", base+"/docker-registries", map[string]string{"name": "x", "host": "ghcr.io", "username": "a"}, nil), 422)
	var g map[string]any
	expect(t, "registre", alice.do("POST", base+"/docker-registries", reg, &g), 201)
	if g["host"] != "ghcr.io" || g["password_enc"] != nil || strings.Contains(toJSON(g), "ghp_secret") {
		t.Fatalf("registre : %v", g)
	}
	expect(t, "doublon", alice.do("POST", base+"/docker-registries", reg, nil), 409)
	var list []store.Registry
	expect(t, "liste par un lecteur", bob.do("GET", base+"/docker-registries", nil, &list), 200)
	if len(list) != 1 {
		t.Fatalf("liste : %+v", list)
	}
	expect(t, "Docker Hub", alice.do("POST", base+"/docker-registries", map[string]string{"name": "Hub", "host": "docker.io", "username": "acme", "password": "dckr_pat"}, nil), 201)

	// Déploiement : identifiants écrits sur le serveur et DOCKER_CONFIG actif pour docker.
	var app map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(nil), &app), 201)
	appID := app["id"].(string)
	var d store.Deployment
	expect(t, "déploiement", alice.do("POST", base+"/applications/"+appID+"/deploy", nil, &d), 202)
	if d = waitDeployment(t, alice, base, appID, d.ID); d.Status != "finished" {
		t.Fatalf("déploiement : %s", d.Error)
	}
	dir := filepath.Join(h.state, "docker-config", f.team)
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg struct {
		Auths map[string]map[string]string `json:"auths"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	want := base64.StdEncoding.EncodeToString([]byte("acme:ghp_secret"))
	if cfg.Auths["ghcr.io"]["auth"] != want || cfg.Auths["https://index.docker.io/v1/"]["auth"] == "" {
		t.Fatalf("config.json : %s", raw)
	}
	if st, _ := os.Stat(filepath.Join(dir, "config.json")); st.Mode().Perm() != 0o600 {
		t.Fatalf("droits : %v", st.Mode())
	}
	if !strings.Contains(h.calls(), "DOCKER_CONFIG="+dir) {
		t.Fatalf("DOCKER_CONFIG absent des appels docker :\n%s", h.calls())
	}
	if strings.Contains(depLogs(t, alice, base, appID, d.ID, true), "ghp_secret") {
		t.Fatal("mot de passe dans les logs")
	}

	// Sans registre, la configuration est retirée au déploiement suivant.
	for _, r := range list {
		expect(t, "suppression par un lecteur", bob.do("DELETE", base+"/docker-registries/"+r.ID, nil, nil), 403)
	}
	expect(t, "liste", alice.do("GET", base+"/docker-registries", nil, &list), 200)
	for _, r := range list {
		expect(t, "suppression", alice.do("DELETE", base+"/docker-registries/"+r.ID, nil, nil), 204)
	}
	expect(t, "redéploiement", alice.do("POST", base+"/applications/"+appID+"/deploy", nil, &d), 202)
	if d = waitDeployment(t, alice, base, appID, d.ID); d.Status != "finished" {
		t.Fatalf("redéploiement : %s", d.Error)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("configuration non retirée : %v", err)
	}
}

func TestLogDrain(t *testing.T) {
	f := newAppFixture(t)
	newFakeHost(t, f)
	alice, base := f.alice, f.base
	bob, _ := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)
	f.e.api.Executors["resource.deploy"] = f.e.api.execResourceDeploy
	srv := base + "/servers/" + f.serverID + "/log-drain"
	var cur map[string]any
	expect(t, "sans log drain", bob.do("GET", srv, nil, &cur), 200)
	if cur["enabled"] != false {
		t.Fatalf("état : %v", cur)
	}
	drain := map[string]any{"type": "newrelic", "license_key": "NRAK-secret", "base_uri": "https://log-api.eu.newrelic.com/log/v1"}
	expect(t, "lecteur", bob.do("PUT", srv, drain, nil), 403)
	expect(t, "type inconnu", alice.do("PUT", srv, map[string]any{"type": "loki"}, nil), 422)
	var resp struct {
		Task store.ServerTask `json:"task"`
	}
	expect(t, "new relic", alice.do("PUT", srv, drain, &resp), 202)
	if resp.Task.Kind != "log_drain_start" {
		t.Fatalf("tâche : %+v", resp.Task)
	}
	if task := waitTask(t, alice, f.team, f.serverID, resp.Task.ID); task.Status != "success" {
		t.Fatalf("démarrage : %s %s", task.Status, task.Error)
	}
	expect(t, "lecture", bob.do("GET", srv, nil, &cur), 200)
	if cfg := cur["config"].(map[string]any); cur["type"] != "newrelic" || cfg["license_key"] != "********" || strings.Contains(toJSON(cur), "NRAK-secret") {
		t.Fatalf("lecture : %v", cur)
	}
	expect(t, "secret conservé", alice.do("PUT", srv, map[string]any{"type": "newrelic", "license_key": "********",
		"base_uri": "https://log-api.newrelic.com/log/v1"}, nil), 202)

	// Application qui active le log drain : pilote fluentd dans son compose.
	var app map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(map[string]any{"settings": map[string]any{"log_drain_enabled": true}}), &app), 201)
	appID := app["id"].(string)
	var d store.Deployment
	expect(t, "déploiement", alice.do("POST", base+"/applications/"+appID+"/deploy", nil, &d), 202)
	if d = waitDeployment(t, alice, base, appID, d.ID); d.Status != "finished" {
		t.Fatalf("déploiement : %s", d.Error)
	}
	if logs := depLogs(t, alice, base, appID, d.ID, true); !strings.Contains(logs, "driver: fluentd") || !strings.Contains(logs, "Logs envoyés au log drain") {
		t.Fatalf("compose sans pilote fluentd :\n%s", logs)
	}

	var stop store.ServerTask
	expect(t, "suppression", alice.do("DELETE", srv, nil, &stop), 202)
	if stop.Kind != "log_drain_stop" {
		t.Fatalf("arrêt : %+v", stop)
	}
	expect(t, "déjà supprimé", alice.do("DELETE", srv, nil, nil), 404)
}
