package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// waitDBTask attend la fin de la dernière action lancée sur la base.
func waitDBTask(t *testing.T, c *client, base, dbID, taskID string) store.ServerTask {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var tasks []store.ServerTask
		expect(t, "actions", c.do("GET", base+"/databases/"+dbID+"/tasks", nil, &tasks), 200)
		for _, task := range tasks {
			if task.ID == taskID && task.Status != "queued" && task.Status != "running" {
				return task
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("action %s non terminée", taskID)
	return store.ServerTask{}
}

func TestDatabases(t *testing.T) {
	f := newAppFixture(t)
	h := newFakeHost(t, f)
	f.e.runner.DatabasesDir = filepath.Join(h.state, "databases")
	f.e.api.Executors["resource.deploy"] = f.e.api.execResourceDeploy
	alice, base := f.alice, f.base
	bob, _ := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)

	db := func(extra map[string]any) map[string]any {
		m := map[string]any{"environment_id": f.envID, "destination_id": f.destID, "type": "postgresql", "name": "Main"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	// Validation
	for name, body := range map[string]map[string]any{
		"type inconnu":               db(map[string]any{"type": "oracle"}),
		"utilisateur invalide":       db(map[string]any{"username": "-x"}),
		"root MySQL":                 db(map[string]any{"type": "mysql", "username": "root"}),
		"mot de passe court":         db(map[string]any{"password": "court"}),
		"root_password postgres":     db(map[string]any{"root_password": "motdepasse-root"}),
		"config non prise en charge": db(map[string]any{"type": "dragonfly", "custom_config": "x"}),
		"script invalide":            db(map[string]any{"init_scripts": []map[string]string{{"filename": "../x.sql", "content": ""}}}),
		"public sans port":           db(map[string]any{"is_public": true}),
		"port réservé":               db(map[string]any{"is_public": true, "public_port": 443}),
		"utilisateur redis":          db(map[string]any{"type": "redis", "username": "admin"}),
		"option postgres sur mysql":  db(map[string]any{"type": "mysql", "settings": map[string]any{"postgres_host_auth_method": "trust"}}),
	} {
		expect(t, name, alice.do("POST", base+"/databases", body, nil), 422)
	}
	expect(t, "lecteur sans droit de création", bob.do("POST", base+"/databases", db(nil), nil), 403)

	var pg map[string]any
	expect(t, "création", alice.do("POST", base+"/databases", db(map[string]any{"database_name": "app", "is_public": true, "public_port": 54320,
		"init_scripts": []map[string]string{{"filename": "01.sql", "content": "CREATE TABLE t (id int);"}}}), &pg), 201)
	id := pg["id"].(string)
	pass, _ := pg["password"].(string)
	if len(pass) != 32 || pg["username"] != "postgres" || pg["image"] != "postgres:17-alpine" || pg["status"] != "stopped" ||
		pg["internal_url"] != "postgres://postgres:"+pass+"@"+id+":5432/app" || pg["public_url"] != "postgres://postgres:"+pass+"@"+f.ssh.Host+":54320/app" {
		t.Fatalf("base créée : %v", pg)
	}
	var storages []store.Storage
	expect(t, "volumes", alice.do("GET", base+"/databases/"+id+"/storages", nil, &storages), 200)
	if len(storages) != 1 || storages[0].Name != "postgresql-data-"+id {
		t.Fatalf("volume par défaut : %+v", storages)
	}
	// Le lecteur voit la base mais pas ses identifiants.
	var seen struct {
		Database map[string]any `json:"database"`
		ServerID string         `json:"server_id"`
	}
	expect(t, "lecture (lecteur)", bob.do("GET", base+"/databases/"+id, nil, &seen), 200)
	if _, leak := seen.Database["password"]; leak || seen.Database["internal_host"] != id || seen.ServerID != f.serverID {
		t.Fatalf("base vue par le lecteur : %v", seen)
	}
	var list []map[string]any
	expect(t, "liste", bob.do("GET", base+"/databases", nil, &list), 200)
	if len(list) != 1 || list[0]["password"] != nil {
		t.Fatalf("liste : %v", list)
	}
	expect(t, "port public déjà pris", alice.do("POST", base+"/databases", db(map[string]any{"name": "Autre", "is_public": true, "public_port": 54320}), nil), 409)
	expect(t, "port SSH", alice.do("POST", base+"/databases", db(map[string]any{"name": "Autre", "ports_mappings": fmt.Sprintf("%d:5432", f.ssh.Port)}), nil), 409)
	expect(t, "port publié deux fois", alice.do("POST", base+"/databases", db(map[string]any{"name": "Autre", "ports_mappings": "6000:5432,6000:5433"}), nil), 409)
	expect(t, "lecteur sans droit de démarrage", bob.do("POST", base+"/databases/"+id+"/start", nil, nil), 403)

	// Variables propres à la base (handlers génériques).
	expect(t, "variable", alice.do("POST", base+"/databases/"+id+"/envs", map[string]any{"key": "TZ", "value": "Europe/Paris"}, nil), 200)

	// Démarrage : scripts réellement exécutés sur le faux hôte.
	var task store.ServerTask
	expect(t, "démarrage", alice.do("POST", base+"/databases/"+id+"/start", nil, &task), 202)
	if task = waitDBTask(t, alice, base, id, task.ID); task.Status != "success" {
		t.Fatalf("démarrage : %s %s\n%s", task.Status, task.Error, task.Output)
	}
	dir := filepath.Join(h.state, "databases", id)
	compose, _ := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
	script, _ := os.ReadFile(filepath.Join(dir, "initdb", "01.sql"))
	nginx, _ := os.ReadFile(filepath.Join(dir, "proxy", "nginx.conf"))
	for _, want := range []string{"container_name: " + id, "POSTGRES_PASSWORD: " + pass, "TZ: Europe/Paris", "POSTGRES_DB: app"} {
		if !strings.Contains(string(compose), want) {
			t.Errorf("compose : %q manquant\n%s", want, compose)
		}
	}
	if string(script) != "CREATE TABLE t (id int);" || !strings.Contains(string(nginx), "listen 54320;") {
		t.Fatalf("fichiers : script %q, nginx %q", script, nginx)
	}
	if cs := strings.Join(h.containers(), ","); !strings.Contains(cs, id) || !strings.Contains(cs, id+"-proxy") {
		t.Fatalf("conteneurs : %s", cs)
	}
	if strings.Contains(task.Output, pass) || !strings.Contains(task.Output, "Base démarrée") {
		t.Fatalf("sortie : %s", task.Output)
	}
	expect(t, "lecture", alice.do("GET", base+"/databases/"+id, nil, &seen), 200)
	if seen.Database["status"] != "running" || seen.Database["started_at"] == nil {
		t.Fatalf("statut : %v", seen.Database["status"])
	}
	var logs struct{ Logs string }
	expect(t, "logs", alice.do("GET", base+"/databases/"+id+"/logs", nil, &logs), 200)
	if !strings.Contains(logs.Logs, "serveur démarré") {
		t.Fatalf("logs : %q", logs.Logs)
	}

	// Exposition publique retirée : le proxy est arrêté sans redémarrer la base.
	var upd map[string]any
	expect(t, "base privée", alice.do("PATCH", base+"/databases/"+id, map[string]any{"is_public": false}, &upd), 200)
	if upd["public_url"] != nil || upd["is_public"] != false {
		t.Fatalf("mise à jour : %v", upd)
	}
	var tasks []store.ServerTask
	expect(t, "actions", alice.do("GET", base+"/databases/"+id+"/tasks", nil, &tasks), 200)
	if len(tasks) != 2 || tasks[0].Kind != "database_proxy" {
		t.Fatalf("tâche du proxy : %+v", tasks)
	}
	waitDBTask(t, alice, base, id, tasks[0].ID)
	if cs := strings.Join(h.containers(), ","); cs != id {
		t.Fatalf("conteneurs après retrait du proxy : %s", cs)
	}
	expect(t, "lecteur sans droit sur le mot de passe", bob.do("PATCH", base+"/databases/"+id, map[string]any{"password": "nouveau-mot-de-passe"}, nil), 403)

	// Redémarrage soumis à approbation, exécuté par l'approbateur.
	dave, _ := register(t, f.e, "dave@example.com")
	invite(t, f.e, alice, dave, f.team, "dave@example.com", rbac.RoleAdmin)
	expect(t, "règle", alice.do("POST", base+"/approval-rules", map[string]any{"name": "Prod", "actions": []string{"resource.deploy"}}, nil), 201)
	var pending struct {
		ApprovalRequest store.ApprovalRequest `json:"approval_request"`
	}
	expect(t, "redémarrage soumis", alice.do("POST", base+"/databases/"+id+"/restart", nil, &pending), 202)
	var appr store.ApprovalRequest
	expect(t, "approbation", dave.do("POST", base+"/approvals/"+pending.ApprovalRequest.ID+"/approve", nil, &appr), 200)
	if appr.Status != "executed" {
		t.Fatalf("approbation : %s %s", appr.Status, appr.Error)
	}
	expect(t, "actions", alice.do("GET", base+"/databases/"+id+"/tasks", nil, &tasks), 200)
	if tasks[0].Kind != "database_restart" {
		t.Fatalf("redémarrage créé par l'approbation : %+v", tasks[0])
	}
	if task = waitDBTask(t, alice, base, id, tasks[0].ID); task.Status != "success" {
		t.Fatalf("redémarrage : %s %s\n%s", task.Status, task.Error, task.Output)
	}
	expect(t, "suppression de la règle", alice.do("DELETE", base+"/approval-rules/"+*pending.ApprovalRequest.RuleID, nil, nil), 204)

	// Arrêt.
	expect(t, "arrêt", alice.do("POST", base+"/databases/"+id+"/stop", nil, &task), 202)
	if task = waitDBTask(t, alice, base, id, task.ID); task.Status != "success" || len(h.containers()) != 0 {
		t.Fatalf("arrêt : %s %s, conteneurs %v", task.Status, task.Output, h.containers())
	}

	// Redis : pas de base ni d'utilisateur configurables, mot de passe dans la commande.
	var rd map[string]any
	expect(t, "redis", alice.do("POST", base+"/databases", db(map[string]any{"type": "redis", "name": "Cache", "password": "mot-de-passe-redis"}), &rd), 201)
	if rd["internal_url"] != "redis://default:mot-de-passe-redis@"+rd["id"].(string)+":6379/0" {
		t.Fatalf("redis : %v", rd)
	}

	// Suppression : conteneurs et fichiers nettoyés, ressources liées supprimées.
	expect(t, "démarrage", alice.do("POST", base+"/databases/"+id+"/start", nil, &task), 202)
	waitDBTask(t, alice, base, id, task.ID)
	expect(t, "suppression", alice.do("DELETE", base+"/databases/"+id+"?delete_volumes=true", nil, nil), 204)
	if _, err := os.Stat(dir); !os.IsNotExist(err) || len(h.containers()) != 0 {
		t.Fatalf("nettoyage : dossier %v, conteneurs %v", err, h.containers())
	}
	if !strings.Contains(h.calls(), "docker volume rm postgresql-data-"+id) {
		t.Fatal("le volume de données doit être supprimé")
	}
	expect(t, "base supprimée", alice.do("GET", base+"/databases/"+id, nil, nil), 404)
}
