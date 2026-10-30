package api

import (
	"slices"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestProjects(t *testing.T) {
	e := newTestEnv(t)
	alice, _ := register(t, e, "alice@example.com")
	bob, bobID := register(t, e, "bob@example.com")
	carol, carolID := register(t, e, "carol@example.com")
	dave, _ := register(t, e, "dave@example.com")
	var acme store.Team
	expect(t, "Acme", alice.do("POST", "/teams", map[string]string{"name": "Acme"}, &acme), 201)
	base := "/teams/" + acme.ID
	invite(t, e, alice, bob, acme.ID, "bob@example.com", rbac.RoleViewer)
	invite(t, e, alice, dave, acme.ID, "dave@example.com", rbac.RoleAdmin)
	expect(t, "rôle Membre", alice.do("POST", base+"/roles", map[string]any{"name": "Membre", "permissions": []string{"team.read"}}, nil), 201)
	invite(t, e, alice, carol, acme.ID, "carol@example.com", "Membre")

	// Projets avec environnement « production » par défaut.
	var web, apiP store.Project
	expect(t, "projet Web", alice.do("POST", base+"/projects", map[string]string{"name": "Web"}, &web), 201)
	expect(t, "projet Api", alice.do("POST", base+"/projects", map[string]string{"name": "Api", "description": "backend"}, &apiP), 201)
	expect(t, "doublon", alice.do("POST", base+"/projects", map[string]string{"name": "web"}, nil), 409)
	var detail struct {
		Project      store.Project       `json:"project"`
		Environments []store.Environment `json:"environments"`
	}
	expect(t, "détail Web", alice.do("GET", base+"/projects/"+web.ID, nil, &detail), 200)
	if len(detail.Environments) != 1 || detail.Environments[0].Name != "production" {
		t.Fatalf("environnement production attendu : %+v", detail.Environments)
	}
	webProd := detail.Environments[0]

	// Bob : lecteur sur l'équipe, développeur sur le projet Web uniquement.
	var projects []store.Project
	expect(t, "projets bob", bob.do("GET", base+"/projects", nil, &projects), 200)
	if len(projects) != 2 {
		t.Fatal("un lecteur voit tous les projets")
	}
	expect(t, "bob crée un environnement (lecteur)", bob.do("POST", base+"/projects/"+web.ID+"/environments", map[string]string{"name": "staging"}, nil), 403)
	expect(t, "développeur sur Web", alice.do("POST", base+"/role-assignments", map[string]any{"user_id": bobID,
		"role_id": roleID(t, alice, acme.ID, rbac.RoleDeveloper), "scope": map[string]string{"type": "project", "id": web.ID}}, nil), 201)
	var staging store.Environment
	expect(t, "bob crée staging dans Web", bob.do("POST", base+"/projects/"+web.ID+"/environments", map[string]string{"name": "staging"}, &staging), 201)
	expect(t, "bob ne crée rien dans Api", bob.do("POST", base+"/projects/"+apiP.ID+"/environments", map[string]string{"name": "staging"}, nil), 403)
	expect(t, "bob renomme Web", bob.do("PATCH", base+"/projects/"+web.ID, map[string]string{"name": "Web", "description": "site"}, nil), 204)
	expect(t, "bob ne renomme pas Api", bob.do("PATCH", base+"/projects/"+apiP.ID, map[string]string{"name": "X"}, nil), 403)

	// Deny sur un environnement précis.
	expect(t, "rôle sans maj env", alice.do("POST", base+"/roles", map[string]any{"name": "Gel", "permissions": []string{"environment.update"}}, nil), 201)
	expect(t, "deny staging", alice.do("POST", base+"/role-assignments", map[string]any{"user_id": bobID, "effect": "deny",
		"role_id": roleID(t, alice, acme.ID, "Gel"), "scope": map[string]string{"type": "environment", "id": staging.ID}}, nil), 201)
	expect(t, "staging gelé", bob.do("PATCH", base+"/projects/"+web.ID+"/environments/"+staging.ID, map[string]string{"name": "staging2"}, nil), 403)
	expect(t, "production modifiable", bob.do("PATCH", base+"/projects/"+web.ID+"/environments/"+webProd.ID, map[string]string{"name": "prod"}, nil), 204)
	var p struct{ Permissions []string }
	expect(t, "permissions sur staging", bob.do("GET", base+"/permissions?scope_type=environment&scope_id="+staging.ID, nil, &p), 200)
	if slices.Contains(p.Permissions, "environment.update") || !slices.Contains(p.Permissions, "resource.deploy") {
		t.Fatalf("permissions effectives sur staging : %v", p.Permissions)
	}
	expect(t, "portée d'un autre projet introuvable", bob.do("PATCH", base+"/projects/"+apiP.ID+"/environments/"+staging.ID, map[string]string{"name": "x"}, nil), 404)

	// Carol : seulement lectrice du projet Api.
	expect(t, "carol lectrice d'Api", alice.do("POST", base+"/role-assignments", map[string]any{"user_id": carolID,
		"role_id": roleID(t, alice, acme.ID, rbac.RoleViewer), "scope": map[string]string{"type": "project", "id": apiP.ID}}, nil), 201)
	expect(t, "projets carol", carol.do("GET", base+"/projects", nil, &projects), 200)
	if len(projects) != 1 || projects[0].ID != apiP.ID {
		t.Fatalf("carol ne voit qu'Api : %+v", projects)
	}
	expect(t, "carol ne voit pas Web", carol.do("GET", base+"/projects/"+web.ID, nil, nil), 403)
	expect(t, "carol voit Api", carol.do("GET", base+"/projects/"+apiP.ID, nil, nil), 200)

	// Variables partagées
	var dbHost, apiKey store.SharedVariable
	expect(t, "clé invalide", alice.do("POST", base+"/variables", map[string]any{"key": "1BAD", "value": "x"}, nil), 422)
	expect(t, "variable équipe", alice.do("POST", base+"/variables", map[string]any{"key": "DB_HOST", "value": "db.internal"}, &dbHost), 201)
	expect(t, "secret", alice.do("POST", base+"/variables", map[string]any{"key": "API_KEY", "value": "s3cr3t", "is_secret": true}, &apiKey), 201)
	expect(t, "doublon", alice.do("POST", base+"/variables", map[string]any{"key": "DB_HOST", "value": "x"}, nil), 409)
	var vars []store.SharedVariable
	expect(t, "liste", alice.do("GET", base+"/variables", nil, &vars), 200)
	if len(vars) != 2 || vars[0].Value != "" || !vars[0].IsSecret || vars[1].Value != "db.internal" {
		t.Fatalf("le secret est masqué, le reste visible : %+v", vars)
	}
	expect(t, "secret non révélable", alice.do("PATCH", base+"/variables/"+apiKey.ID, map[string]any{"is_secret": false}, nil), 422)
	expect(t, "lecteur sans variable.read", bob.do("GET", base+"/variables", nil, nil), 403)
	envVar := map[string]any{"key": "NODE_ENV", "value": "production", "scope": map[string]string{"type": "environment", "id": webProd.ID}}
	expect(t, "variable d'environnement par bob (dev sur Web)", bob.do("POST", base+"/variables", envVar, nil), 403) // variable.manage absent du rôle développeur
	expect(t, "variable d'environnement", alice.do("POST", base+"/variables", envVar, nil), 201)
	expect(t, "bob lit les variables de Web/prod", bob.do("GET", base+"/variables?scope_type=environment&scope_id="+webProd.ID, nil, &vars), 200)
	if len(vars) != 1 || vars[0].Value != "production" {
		t.Fatalf("variables d'environnement : %+v", vars)
	}
	shared, err := e.st.SharedVariablesFor(t.Context(), acme.ID, web.ID, webProd.ID, "")
	if err != nil || shared["team.DB_HOST"] == nil || shared["environment.NODE_ENV"] == nil || shared["team.API_KEY"] == nil {
		t.Fatalf("résolution des variables partagées : %v %v", shared, err)
	}

	// Clonage vers un autre projet : variables d'environnement copiées.
	var preview store.Environment
	expect(t, "bob ne clone pas vers Api", bob.do("POST", base+"/projects/"+web.ID+"/environments/"+webProd.ID+"/clone", map[string]string{"name": "preview", "project_id": apiP.ID}, nil), 403)
	expect(t, "clonage", alice.do("POST", base+"/projects/"+web.ID+"/environments/"+webProd.ID+"/clone", map[string]string{"name": "preview", "project_id": apiP.ID}, &preview), 201)
	if preview.ProjectID != apiP.ID {
		t.Fatal("clone dans le projet cible")
	}
	expect(t, "variables du clone", alice.do("GET", base+"/variables?scope_type=environment&scope_id="+preview.ID, nil, &vars), 200)
	if len(vars) != 1 || vars[0].Key != "NODE_ENV" {
		t.Fatalf("variables clonées : %+v", vars)
	}

	// Suppression d'environnement soumise à approbation.
	expect(t, "règle", alice.do("POST", base+"/approval-rules", map[string]any{"name": "Envs", "actions": []string{"environment.delete"}}, nil), 201)
	var pending struct {
		ApprovalRequest store.ApprovalRequest `json:"approval_request"`
	}
	expect(t, "suppression soumise", alice.do("DELETE", base+"/projects/"+apiP.ID+"/environments/"+preview.ID, nil, &pending), 202)
	var got store.ApprovalRequest
	expect(t, "approbation", dave.do("POST", base+"/approvals/"+pending.ApprovalRequest.ID+"/approve", nil, &got), 200)
	if got.Status != "executed" {
		t.Fatalf("exécution : %s %s", got.Status, got.Error)
	}
	expect(t, "environnement supprimé", alice.do("GET", base+"/projects/"+apiP.ID+"/environments/"+preview.ID, nil, nil), 404)
	expect(t, "variables supprimées avec", alice.do("GET", base+"/variables?scope_type=environment&scope_id="+preview.ID, nil, nil), 404)

	// Suppression de projet : environnements, variables et attributions de rôle supprimés.
	expect(t, "suppression Web", alice.do("DELETE", base+"/projects/"+web.ID, nil, nil), 204)
	expect(t, "Web supprimé", alice.do("GET", base+"/projects/"+web.ID, nil, nil), 404)
	var assignments []store.RoleAssignment
	expect(t, "attributions", alice.do("GET", base+"/role-assignments", nil, &assignments), 200)
	if slices.ContainsFunc(assignments, func(a store.RoleAssignment) bool { return a.Scope.ID == web.ID || a.Scope.ID == staging.ID }) {
		t.Fatal("les attributions sur le projet supprimé doivent disparaître")
	}

	// Tags
	var tag store.Tag
	expect(t, "couleur invalide", alice.do("POST", base+"/tags", map[string]string{"name": "prod", "color": "rouge"}, nil), 422)
	expect(t, "tag", alice.do("POST", base+"/tags", map[string]string{"name": "prod", "color": "#ff0000"}, &tag), 201)
	expect(t, "tag doublon", alice.do("POST", base+"/tags", map[string]string{"name": "PROD"}, nil), 409)
	expect(t, "tag par un lecteur", bob.do("POST", base+"/tags", map[string]string{"name": "x"}, nil), 403)
	expect(t, "renommage tag", alice.do("PATCH", base+"/tags/"+tag.ID, map[string]string{"name": "production", "color": "#00ff00"}, nil), 204)
	var tags []store.Tag
	expect(t, "tags", bob.do("GET", base+"/tags", nil, &tags), 200)
	if len(tags) != 1 || tags[0].Name != "production" {
		t.Fatalf("tags : %+v", tags)
	}
	expect(t, "suppression tag", alice.do("DELETE", base+"/tags/"+tag.ID, nil, nil), 204)
}
