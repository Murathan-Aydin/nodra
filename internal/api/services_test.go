package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

type serviceResp struct {
	Service         store.Service          `json:"service"`
	Apps            []store.ServiceApp     `json:"apps"`
	Task            *store.ServerTask      `json:"task"`
	ApprovalRequest *store.ApprovalRequest `json:"approval_request"`
}

func TestServicesAPI(t *testing.T) {
	f := newAppFixture(t)
	h := newFakeHost(t, f)
	f.e.runner.ServicesDir = filepath.Join(h.state, "services")
	f.e.api.Executors["resource.deploy"] = f.e.api.execResourceDeploy // l'environnement de test en installe un faux
	alice, base := f.alice, f.base
	bob, _ := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)

	// Catalogue
	var cat struct {
		Templates  []map[string]any `json:"templates"`
		Categories []string         `json:"categories"`
	}
	expect(t, "catalogue", bob.do("GET", "/service-templates?q=n8n", nil, &cat), 200)
	if len(cat.Templates) == 0 || cat.Templates[0]["compose"] != nil || len(cat.Categories) == 0 {
		t.Fatalf("catalogue : %+v", cat)
	}
	var tpl map[string]any
	expect(t, "modèle", bob.do("GET", "/service-templates/n8n", nil, &tpl), 200)
	expect(t, "modèle inconnu", bob.do("GET", "/service-templates/inexistant", nil, nil), 404)

	// Création depuis le catalogue avec démarrage immédiat.
	req := map[string]any{"environment_id": f.envID, "destination_id": f.destID, "template": "n8n", "instant_deploy": true}
	expect(t, "lecteur sans resource.create", bob.do("POST", base+"/services", req, nil), 403)
	var n8n serviceResp
	expect(t, "service n8n", alice.do("POST", base+"/services", req, &n8n), 201)
	if n8n.Service.Name != "N8n" || n8n.Task == nil {
		t.Fatalf("création : %+v", n8n)
	}
	var app *store.ServiceApp
	for i := range n8n.Apps {
		if n8n.Apps[i].Name == "n8n" {
			app = &n8n.Apps[i]
		}
	}
	if app == nil || app.URLVar != "N8N" || app.Port != "5678" || len(app.Domains) != 1 || !strings.HasSuffix(app.Domains[0], ".127.0.0.1.sslip.io:5678") {
		t.Fatalf("sous-service exposé : %+v", n8n.Apps)
	}
	task := waitTask(t, alice, f.team, f.serverID, n8n.Task.ID)
	if task.Status != "success" || !strings.Contains(task.Output, "Service démarré") {
		t.Fatalf("démarrage : %s %s\n%s", task.Status, task.Error, task.Output)
	}
	envFile, _ := os.ReadFile(filepath.Join(h.state, "services", n8n.Service.ID, ".env"))
	domainHost := strings.TrimSuffix(strings.TrimPrefix(app.Domains[0], "http://"), ":5678")
	if !strings.Contains(string(envFile), `SERVICE_URL_N8N_5678="http://`+domainHost+`"`) {
		t.Fatalf(".env : %s", envFile)
	}
	var got serviceResp
	expect(t, "statut", alice.do("GET", base+"/services/"+n8n.Service.ID, nil, &got), 200)
	if got.Service.Status != "running" {
		t.Fatalf("statut : %s", got.Service.Status)
	}

	// Compose personnalisé : secrets générés, valeurs par défaut, conservation à la modification.
	compose := "services:\n  web:\n    image: nginx\n    environment:\n      - SERVICE_URL_WEB_80\n      - DB_PASS=${SERVICE_PASSWORD_PG}\n      - TITLE=${TITLE:-Mon site}\n"
	var custom serviceResp
	expect(t, "compose invalide", alice.do("POST", base+"/services", map[string]any{"environment_id": f.envID, "destination_id": f.destID, "name": "X", "docker_compose_raw": "services: ["}, nil), 422)
	expect(t, "compose personnalisé", alice.do("POST", base+"/services", map[string]any{"environment_id": f.envID, "destination_id": f.destID,
		"name": "Site", "docker_compose_raw": compose}, &custom), 201)
	sid := custom.Service.ID
	envs := func() map[string]string {
		var list []store.EnvVar
		expect(t, "variables", alice.do("GET", base+"/services/"+sid+"/envs", nil, &list), 200)
		m := map[string]string{}
		for _, v := range list {
			m[v.Key] = v.Value
		}
		return m
	}
	v1 := envs()
	if len(v1["SERVICE_PASSWORD_PG"]) != 24 || v1["TITLE"] != "Mon site" {
		t.Fatalf("variables générées : %v", v1)
	}
	expect(t, "lecteur sans env.read", bob.do("GET", base+"/services/"+sid+"/envs", nil, nil), 403)
	compose2 := compose + "      - TOKEN=${SERVICE_HEX_32_TOKEN}\n  worker:\n    image: busybox\n    exclude_from_hc: true\n"
	expect(t, "modification du compose", alice.do("PATCH", base+"/services/"+sid, map[string]any{"docker_compose_raw": compose2}, &got), 200)
	v2 := envs()
	if v2["SERVICE_PASSWORD_PG"] != v1["SERVICE_PASSWORD_PG"] || len(v2["SERVICE_HEX_32_TOKEN"]) != 32 || len(got.Apps) != 2 || !got.Apps[1].ExcludeFromStatus {
		t.Fatalf("après modification : %v %+v", v2, got.Apps)
	}

	// Domaines d'un sous-service, conflit avec une application.
	expect(t, "domaine", alice.do("PATCH", base+"/services/"+sid+"/apps/web", map[string]any{"domains": []string{"https://site.example.com"}}, nil), 200)
	expect(t, "conflit", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Autre", "domains": []string{"https://site.example.com"}}), nil), 409)
	expect(t, "sous-service inconnu", alice.do("PATCH", base+"/services/"+sid+"/apps/nope", map[string]any{"port": "80"}, nil), 404)

	// Actions : démarrage soumis à approbation, redémarrage d'un conteneur, arrêt, logs.
	dave, _ := register(t, f.e, "dave@example.com")
	invite(t, f.e, alice, dave, f.team, "dave@example.com", rbac.RoleAdmin)
	expect(t, "règle", alice.do("POST", base+"/approval-rules", map[string]any{"name": "Svc", "actions": []string{"resource.deploy"},
		"scope": map[string]string{"type": "resource", "id": sid}}, nil), 201)
	var pending struct {
		ApprovalRequest store.ApprovalRequest `json:"approval_request"`
	}
	expect(t, "démarrage soumis", alice.do("POST", base+"/services/"+sid+"/start", nil, &pending), 202)
	var appr store.ApprovalRequest
	expect(t, "approbation", dave.do("POST", base+"/approvals/"+pending.ApprovalRequest.ID+"/approve", nil, &appr), 200)
	if appr.Status != "executed" {
		t.Fatalf("approbation : %s %s", appr.Status, appr.Error)
	}
	var tasks []store.ServerTask
	expect(t, "tâches", alice.do("GET", base+"/services/"+sid+"/tasks", nil, &tasks), 200)
	if len(tasks) != 1 || waitTask(t, alice, f.team, f.serverID, tasks[0].ID).Status != "success" {
		t.Fatalf("tâches : %+v", tasks)
	}
	expect(t, "suppression règle", alice.do("DELETE", base+"/approval-rules/"+*appr.RuleID, nil, nil), 204)
	var tk store.ServerTask
	expect(t, "redémarrage d'un conteneur", alice.do("POST", base+"/services/"+sid+"/apps/web/restart", nil, &tk), 202)
	if tk = waitTask(t, alice, f.team, f.serverID, tk.ID); tk.Status != "success" || !strings.Contains(tk.Output, "restart") {
		t.Fatalf("redémarrage : %s %s", tk.Status, tk.Output)
	}
	var logs struct{ Logs string }
	expect(t, "logs", alice.do("GET", base+"/services/"+sid+"/logs?app=web", nil, &logs), 200)
	expect(t, "arrêt", alice.do("POST", base+"/services/"+sid+"/stop", nil, &tk), 202)
	if tk = waitTask(t, alice, f.team, f.serverID, tk.ID); tk.Status != "success" {
		t.Fatalf("arrêt : %s %s", tk.Status, tk.Error)
	}
	expect(t, "statut", alice.do("GET", base+"/services/"+sid, nil, &got), 200)
	if got.Service.Status != "stopped" {
		t.Fatalf("statut après arrêt : %s", got.Service.Status)
	}

	// Liste, suppression avec volumes.
	var list []store.Service
	expect(t, "liste", bob.do("GET", base+"/services?environment_id="+f.envID, nil, &list), 200)
	if len(list) != 2 {
		t.Fatalf("2 services : %d", len(list))
	}
	expect(t, "suppression", alice.do("DELETE", base+"/services/"+n8n.Service.ID+"?delete_volumes=true", nil, nil), 204)
	if _, err := os.Stat(filepath.Join(h.state, "services", n8n.Service.ID)); !os.IsNotExist(err) {
		t.Fatal("dossier du service supprimé")
	}
	if !strings.Contains(h.calls(), "docker compose -p "+n8n.Service.ID+" down --remove-orphans -v") {
		t.Fatal("volumes supprimés avec le service")
	}
}
