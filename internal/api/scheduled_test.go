package api

import (
	"strings"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestScheduledTasks(t *testing.T) {
	f := newAppFixture(t)
	h := newFakeHost(t, f)
	alice, base := f.alice, f.base
	bob, bobID := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)

	var app map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(nil), &app), 201)
	appID := app["id"].(string)
	task := map[string]any{"resource_id": appID, "name": "Purge du cache", "command": "php artisan cache:clear", "frequency": "*/5 * * * *"}
	expect(t, "lecteur sans resource.terminal", bob.do("POST", base+"/scheduled-tasks", task, nil), 403)
	expect(t, "cron invalide", alice.do("POST", base+"/scheduled-tasks", map[string]any{"resource_id": appID, "name": "x", "command": "ls", "frequency": "souvent"}, nil), 422)
	expect(t, "timeout invalide", alice.do("POST", base+"/scheduled-tasks", map[string]any{"resource_id": appID, "name": "x", "command": "ls", "frequency": "@daily", "timeout": 5}, nil), 422)
	var st store.ScheduledTask
	expect(t, "tâche", alice.do("POST", base+"/scheduled-tasks", task, &st), 201)
	var list []store.ScheduledTask
	expect(t, "liste par un lecteur", bob.do("GET", base+"/scheduled-tasks?resource_id="+appID, nil, &list), 200)
	if len(list) != 1 || list[0].Timeout != 3600 || !list[0].Enabled {
		t.Fatalf("liste : %+v", list)
	}

	// Sans conteneur en cours : échec explicite.
	var tk store.ServerTask
	expect(t, "exécution", alice.do("POST", base+"/scheduled-tasks/"+st.ID+"/run", nil, &tk), 202)
	if tk = waitTask(t, alice, f.team, f.serverID, tk.ID); tk.Status != "failed" || !strings.Contains(tk.Error, "aucun conteneur") {
		t.Fatalf("sans conteneur : %s %s", tk.Status, tk.Error)
	}

	// Après déploiement : commande exécutée dans le conteneur.
	f.e.api.Executors["resource.deploy"] = f.e.api.execResourceDeploy
	var d store.Deployment
	expect(t, "déploiement", alice.do("POST", base+"/applications/"+appID+"/deploy", nil, &d), 202)
	if d = waitDeployment(t, alice, base, appID, d.ID); d.Status != "finished" {
		t.Fatalf("déploiement : %s", d.Error)
	}
	expect(t, "exécution", alice.do("POST", base+"/scheduled-tasks/"+st.ID+"/run", nil, &tk), 202)
	if tk = waitTask(t, alice, f.team, f.serverID, tk.ID); tk.Status != "success" || !strings.Contains(tk.Output, "php artisan cache:clear") {
		t.Fatalf("exécution : %s %s\n%s", tk.Status, tk.Error, tk.Output)
	}
	if !strings.Contains(h.calls(), "docker exec "+appID) {
		t.Fatal("commande lancée dans le conteneur de l'application")
	}
	var execs []store.ServerTask
	expect(t, "historique", alice.do("GET", base+"/scheduled-tasks/"+st.ID+"/executions", nil, &execs), 200)
	if len(execs) != 2 {
		t.Fatalf("2 exécutions : %d", len(execs))
	}

	// Planificateur : due une seule fois, puis désactivée.
	if err := f.e.jobs.RunSchedules(t.Context(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := f.e.jobs.RunSchedules(t.Context(), time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	all, _ := f.e.st.TasksWithParam(t.Context(), "scheduled_task_id", st.ID, 100)
	scheduled := 0
	for _, x := range all {
		if x.CreatedBy == nil {
			scheduled++
		}
	}
	if scheduled != 1 {
		t.Fatalf("1 exécution planifiée attendue, %d", scheduled)
	}
	expect(t, "désactivation", alice.do("PATCH", base+"/scheduled-tasks/"+st.ID, map[string]any{"enabled": false}, nil), 200)

	// Droits : développeur sur l'application uniquement.
	expect(t, "développeur", alice.do("POST", base+"/role-assignments", map[string]any{"user_id": bobID,
		"role_id": roleID(t, alice, f.team, rbac.RoleDeveloper), "scope": map[string]string{"type": "resource", "id": appID}}, nil), 201)
	expect(t, "bob modifie", bob.do("PATCH", base+"/scheduled-tasks/"+st.ID, map[string]any{"command": "ls"}, nil), 200)
	expect(t, "suppression", alice.do("DELETE", base+"/scheduled-tasks/"+st.ID, nil, nil), 204)
}
