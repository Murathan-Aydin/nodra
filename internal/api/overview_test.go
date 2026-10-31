package api

import (
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestOverviewAndBulkDeploy(t *testing.T) {
	f := newAppFixture(t)
	newFakeHost(t, f)
	alice, base := f.alice, f.base
	bob, bobID := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)
	f.e.api.Executors["resource.deploy"] = f.e.api.execResourceDeploy

	var app, other map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(map[string]any{"domains": []string{"https://site.example.com"}}), &app), 201)
	expect(t, "autre application", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Docs"}), &other), 201)
	appID, otherID := app["id"].(string), other["id"].(string)
	var db store.Database
	expect(t, "base", alice.do("POST", base+"/databases", map[string]any{"environment_id": f.envID, "destination_id": f.destID,
		"type": "postgresql", "name": "Main"}, &db), 201)
	var tag store.Tag
	expect(t, "tag", alice.do("POST", base+"/tags", map[string]string{"name": "Prod"}, &tag), 201)
	expect(t, "tag application", alice.do("PUT", base+"/applications/"+appID+"/tags", map[string]any{"tag_ids": []string{tag.ID}}, nil), 200)
	expect(t, "tag base", alice.do("PUT", base+"/databases/"+db.ID+"/tags", map[string]any{"tag_ids": []string{tag.ID}}, nil), 200)

	var res []store.ResourceSummary
	expect(t, "ressources", alice.do("GET", base+"/resources", nil, &res), 200)
	if len(res) != 3 {
		t.Fatalf("ressources : %+v", res)
	}
	expect(t, "par tag", alice.do("GET", base+"/resources?tag=prod&type=database", nil, &res), 200)
	if len(res) != 1 || res[0].ID != db.ID || res[0].Tags[0] != "Prod" || res[0].ProjectID == "" || res[0].ServerID != f.serverID {
		t.Fatalf("filtre : %+v", res)
	}
	expect(t, "type invalide", alice.do("GET", base+"/resources?type=vm", nil, nil), 400)
	// Bob ne voit pas l'application qui lui est refusée.
	expect(t, "refus", alice.do("POST", base+"/role-assignments", map[string]any{"user_id": bobID,
		"role_id": roleID(t, alice, f.team, rbac.RoleViewer), "scope": map[string]string{"type": "resource", "id": appID}, "effect": "deny"}, nil), 201)
	expect(t, "ressources de bob", bob.do("GET", base+"/resources", nil, &res), 200)
	if len(res) != 2 {
		t.Fatalf("filtrage par droits : %+v", res)
	}
	var domains []map[string]string
	expect(t, "domaines du serveur", alice.do("GET", base+"/servers/"+f.serverID+"/domains", nil, &domains), 200)
	// Docs a reçu un domaine sslip.io généré ; triés par domaine.
	if len(domains) != 2 || domains[1]["domain"] != "https://site.example.com" || domains[1]["resource_id"] != appID {
		t.Fatalf("domaines : %v", domains)
	}
	expect(t, "ressources du serveur", alice.do("GET", base+"/servers/"+f.serverID+"/resources", nil, &res), 200)
	if len(res) != 3 {
		t.Fatalf("ressources du serveur : %+v", res)
	}

	// Déploiement groupé par tag et par identifiant.
	expect(t, "sans cible", alice.do("POST", base+"/deploy", map[string]any{}, nil), 422)
	expect(t, "identifiant invalide", alice.do("POST", base+"/deploy?uuid=nope", nil, nil), 422)
	var bulk struct {
		Results []struct {
			ResourceID string         `json:"resource_id"`
			Type       string         `json:"type"`
			Status     int            `json:"status"`
			Response   map[string]any `json:"response"`
		} `json:"results"`
	}
	expect(t, "lecteur", bob.do("POST", base+"/deploy?tag=prod", nil, &bulk), 200)
	for _, r := range bulk.Results {
		if r.Status != 403 {
			t.Fatalf("lecteur : %+v", bulk.Results)
		}
	}
	expect(t, "déploiement groupé", alice.do("POST", base+"/deploy?tag=PROD&uuid="+otherID+",00000000-0000-0000-0000-000000000000",
		map[string]any{"force_rebuild": true}, &bulk), 200)
	if len(bulk.Results) != 4 {
		t.Fatalf("résultats : %+v", bulk.Results)
	}
	got := map[string]int{}
	var depID string
	for _, r := range bulk.Results {
		got[r.ResourceID] = r.Status
		if r.ResourceID == appID {
			depID, _ = r.Response["id"].(string)
		}
	}
	if got[appID] != 202 || got[otherID] != 202 || got[db.ID] != 202 || got["00000000-0000-0000-0000-000000000000"] != 404 || depID == "" {
		t.Fatalf("statuts : %+v", bulk.Results)
	}
	if d := waitDeployment(t, alice, base, appID, depID); d.Status != "finished" || !d.ForceRebuild {
		t.Fatalf("déploiement : %+v", d)
	}
	var deps []store.Deployment
	expect(t, "déploiements de l'équipe", alice.do("GET", base+"/deployments", nil, &deps), 200)
	if len(deps) != 2 {
		t.Fatalf("déploiements : %+v", deps)
	}
	expect(t, "déploiements de bob", bob.do("GET", base+"/deployments", nil, &deps), 200)
	if len(deps) != 1 || deps[0].ApplicationID != otherID {
		t.Fatalf("déploiements visibles par bob : %+v", deps)
	}
	expect(t, "statut invalide", alice.do("GET", base+"/deployments?status=nope", nil, nil), 400)

	// Déplacement vers un autre environnement.
	var env store.Environment
	expect(t, "environnement", alice.do("POST", base+"/projects/"+f.projectID+"/environments", map[string]string{"name": "staging"}, &env), 201)
	expect(t, "déplacement par un lecteur", bob.do("POST", base+"/databases/"+db.ID+"/move", map[string]string{"environment_id": env.ID}, nil), 403)
	expect(t, "environnement inconnu", alice.do("POST", base+"/databases/"+db.ID+"/move",
		map[string]string{"environment_id": "00000000-0000-0000-0000-000000000000"}, nil), 404)
	expect(t, "déplacement", alice.do("POST", base+"/databases/"+db.ID+"/move", map[string]string{"environment_id": env.ID}, nil), 204)
	expect(t, "après déplacement", alice.do("GET", base+"/resources?environment_id="+env.ID, nil, &res), 200)
	if len(res) != 1 || res[0].ID != db.ID || res[0].EnvironmentName != "staging" {
		t.Fatalf("déplacement : %+v", res)
	}
	var v map[string]string
	expect(t, "version", alice.do("GET", "/version", nil, &v), 200)
	if v["version"] != "dev" {
		t.Fatalf("version : %v", v)
	}
}
