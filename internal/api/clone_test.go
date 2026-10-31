package api

import (
	"strings"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestCloneResources(t *testing.T) {
	f := newAppFixture(t)
	alice, base := f.alice, f.base
	bob, _ := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)

	var app map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(map[string]any{"domains": []string{"https://site.example.com"}}), &app), 201)
	appID := app["id"].(string)
	expect(t, "variable", alice.do("POST", base+"/applications/"+appID+"/envs", map[string]any{"key": "MODE", "value": "prod"}, nil), 200)
	expect(t, "secret", alice.do("POST", base+"/applications/"+appID+"/envs", map[string]any{"key": "API_KEY", "value": "s3cr3t", "is_secret": true}, nil), 200)
	var vol store.Storage
	expect(t, "volume", alice.do("POST", base+"/applications/"+appID+"/storages", map[string]any{"kind": "volume", "mount_path": "/data"}, &vol), 201)
	var tag store.Tag
	expect(t, "tag", alice.do("POST", base+"/tags", map[string]string{"name": "web"}, &tag), 201)
	expect(t, "tag appliqué", alice.do("PUT", base+"/applications/"+appID+"/tags", map[string]any{"tag_ids": []string{tag.ID}}, nil), 200)
	expect(t, "tâche", alice.do("POST", base+"/scheduled-tasks", map[string]any{"resource_id": appID, "name": "Cache", "command": "ls",
		"frequency": "@daily"}, nil), 201)

	expect(t, "lecteur", bob.do("POST", base+"/applications/"+appID+"/clone", nil, nil), 403)
	var created map[string]string
	expect(t, "clone", alice.do("POST", base+"/applications/"+appID+"/clone", nil, &created), 201)
	cloneID := created["id"]
	var got struct {
		Application map[string]any `json:"application"`
	}
	expect(t, "copie", alice.do("GET", base+"/applications/"+cloneID, nil, &got), 200)
	c := got.Application
	domains, _ := c["domains"].([]any)
	if c["name"] != "Site (copie)" || c["status"] != "stopped" || len(domains) != 1 || domains[0] == "https://site.example.com" ||
		!strings.Contains(domains[0].(string), "sslip.io") {
		t.Fatalf("copie : %v", c)
	}
	var envs []store.EnvVar
	expect(t, "variables", alice.do("GET", base+"/applications/"+cloneID+"/envs", nil, &envs), 200)
	byKey := map[string]store.EnvVar{}
	for _, e := range envs {
		byKey[e.Key] = e
	}
	if len(envs) != 2 || byKey["MODE"].Value != "prod" || !byKey["API_KEY"].IsSecret {
		t.Fatalf("variables : %+v", envs)
	}
	var stg []store.Storage
	expect(t, "stockages", alice.do("GET", base+"/applications/"+cloneID+"/storages", nil, &stg), 200)
	if len(stg) != 1 || stg[0].Name == vol.Name || stg[0].MountPath != "/data" || !strings.HasPrefix(stg[0].Name, strings.ReplaceAll(cloneID, "-", "")[:12]) {
		t.Fatalf("volume renommé : %+v (original %s)", stg, vol.Name)
	}
	var res []store.ResourceSummary
	expect(t, "tags", alice.do("GET", base+"/resources?tag=web", nil, &res), 200)
	if len(res) != 2 {
		t.Fatalf("tags copiés : %+v", res)
	}
	var tasks []store.ScheduledTask
	expect(t, "tâches", alice.do("GET", base+"/scheduled-tasks?resource_id="+cloneID, nil, &tasks), 200)
	if len(tasks) != 1 || tasks[0].Enabled {
		t.Fatalf("tâche copiée désactivée : %+v", tasks)
	}

	// Vers un autre environnement, avec un nom choisi.
	var env store.Environment
	expect(t, "environnement", alice.do("POST", base+"/projects/"+f.projectID+"/environments", map[string]string{"name": "staging"}, &env), 201)
	expect(t, "clone en staging", alice.do("POST", base+"/applications/"+appID+"/clone", map[string]string{"name": "Site staging", "environment_id": env.ID}, &created), 201)
	expect(t, "ressources de staging", alice.do("GET", base+"/resources?environment_id="+env.ID, nil, &res), 200)
	if len(res) != 1 || res[0].Name != "Site staging" {
		t.Fatalf("staging : %+v", res)
	}
	expect(t, "destination inconnue", alice.do("POST", base+"/applications/"+appID+"/clone",
		map[string]string{"destination_id": "00000000-0000-0000-0000-000000000000"}, nil), 422)

	// Base : volumes de données renommés, exposition publique retirée.
	var db store.Database
	expect(t, "base", alice.do("POST", base+"/databases", map[string]any{"environment_id": f.envID, "destination_id": f.destID,
		"type": "postgresql", "name": "Main"}, &db), 201)
	expect(t, "clone de la base", alice.do("POST", base+"/databases/"+db.ID+"/clone", nil, &created), 201)
	expect(t, "stockages de la copie", alice.do("GET", base+"/databases/"+created["id"]+"/storages", nil, &stg), 200)
	if len(stg) == 0 || !strings.Contains(stg[0].Name, created["id"]) || strings.Contains(stg[0].Name, db.ID) {
		t.Fatalf("volumes de la base : %+v", stg)
	}

	// Service : sous-services copiés avec de nouveaux domaines.
	var svc serviceResp
	expect(t, "service", alice.do("POST", base+"/services", map[string]any{"environment_id": f.envID, "destination_id": f.destID,
		"template": "uptime-kuma"}, &svc), 201)
	expect(t, "clone du service", alice.do("POST", base+"/services/"+svc.Service.ID+"/clone", nil, &created), 201)
	var copy serviceResp
	expect(t, "service copié", alice.do("GET", base+"/services/"+created["id"], nil, &copy), 200)
	if len(copy.Apps) != len(svc.Apps) || copy.Service.Name != svc.Service.Name+" (copie)" {
		t.Fatalf("service copié : %+v", copy)
	}
	for i := range copy.Apps {
		if len(svc.Apps[i].Domains) > 0 && (len(copy.Apps[i].Domains) != 1 || copy.Apps[i].Domains[0] == svc.Apps[i].Domains[0]) {
			t.Fatalf("domaines des sous-services : %+v / %+v", copy.Apps[i], svc.Apps[i])
		}
	}
}

func TestCloneEnvironmentWithResources(t *testing.T) {
	f := newAppFixture(t)
	alice, base := f.alice, f.base
	expect(t, "application", alice.do("POST", base+"/applications", f.app(nil), nil), 201)
	expect(t, "base", alice.do("POST", base+"/databases", map[string]any{"environment_id": f.envID, "destination_id": f.destID,
		"type": "redis", "name": "Cache"}, nil), 201)
	var env store.Environment
	expect(t, "clone sans ressources", alice.do("POST", base+"/projects/"+f.projectID+"/environments/"+f.envID+"/clone",
		map[string]any{"name": "vide"}, &env), 201)
	var res []store.ResourceSummary
	expect(t, "vide", alice.do("GET", base+"/resources?environment_id="+env.ID, nil, &res), 200)
	if len(res) != 0 {
		t.Fatalf("aucune ressource attendue : %+v", res)
	}
	expect(t, "clone avec ressources", alice.do("POST", base+"/projects/"+f.projectID+"/environments/"+f.envID+"/clone",
		map[string]any{"name": "staging", "with_resources": true}, &env), 201)
	expect(t, "ressources copiées", alice.do("GET", base+"/resources?environment_id="+env.ID, nil, &res), 200)
	if len(res) != 2 || res[0].Status != "stopped" {
		t.Fatalf("ressources de staging : %+v", res)
	}
}
