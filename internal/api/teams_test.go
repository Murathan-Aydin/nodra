package api

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func register(t *testing.T, e *testEnv, email string) (*client, string) {
	t.Helper()
	c := newClient(t, e.srv)
	var u store.User
	expect(t, "inscription "+email, c.do("POST", "/auth/register", map[string]string{"email": email, "password": "motdepasse1"}, &u), 201)
	return c, u.ID
}

type teamInfo struct {
	Team        store.Team `json:"team"`
	Permissions []string   `json:"permissions"`
	IsOwner     bool       `json:"is_owner"`
}

func perms(t *testing.T, c *client, teamID string) []string {
	t.Helper()
	var p struct{ Permissions []string }
	expect(t, "permissions", c.do("GET", "/teams/"+teamID+"/permissions", nil, &p), 200)
	return p.Permissions
}

func roleID(t *testing.T, c *client, teamID, key string) string {
	t.Helper()
	var roles []store.Role
	expect(t, "rôles", c.do("GET", "/teams/"+teamID+"/roles", nil, &roles), 200)
	for _, r := range roles {
		if r.Key == key || (!r.System && r.Name == key) {
			return r.ID
		}
	}
	t.Fatalf("rôle %s introuvable", key)
	return ""
}

func invite(t *testing.T, e *testEnv, admin, invitee *client, teamID, email, role string) {
	t.Helper()
	expect(t, "invitation "+email, admin.do("POST", "/teams/"+teamID+"/invitations",
		map[string]any{"email": email, "role_id": roleID(t, admin, teamID, role)}, nil), 201)
	expect(t, "acceptation "+email, invitee.do("POST", "/invitations/accept", map[string]string{"token": e.mailer.lastInvite}, nil), 200)
}

func TestTeams(t *testing.T) {
	e := newTestEnv(t)
	alice, _ := register(t, e, "alice@example.com")
	bob, bobID := register(t, e, "bob@example.com")
	carol, carolID := register(t, e, "carol@example.com")

	// Équipe personnelle créée à l'inscription.
	var teams []store.Team
	expect(t, "équipes alice", alice.do("GET", "/teams", nil, &teams), 200)
	if len(teams) != 1 || !teams[0].Personal {
		t.Fatalf("une équipe personnelle attendue, obtenu %+v", teams)
	}
	personal := teams[0].ID
	expect(t, "suppression équipe perso", alice.do("DELETE", "/teams/"+personal, nil, nil), 409)

	// Organisation
	var acme store.Team
	expect(t, "création Acme", alice.do("POST", "/teams", map[string]string{"name": "Acme"}, &acme), 201)
	var info teamInfo
	expect(t, "Acme alice", alice.do("GET", "/teams/"+acme.ID, nil, &info), 200)
	if !info.IsOwner || !slices.Contains(info.Permissions, "team.delete") {
		t.Fatal("le créateur est propriétaire")
	}
	expect(t, "Acme invisible pour bob", bob.do("GET", "/teams/"+acme.ID, nil, nil), 404)
	expect(t, "id malformé", alice.do("GET", "/teams/pas-un-uuid", nil, nil), 404)

	// Invitations : liée à l'adresse e-mail.
	expect(t, "invitation bob", alice.do("POST", "/teams/"+acme.ID+"/invitations",
		map[string]any{"email": "bob@example.com", "role_id": roleID(t, alice, acme.ID, rbac.RoleDeveloper)}, nil), 201)
	var preview struct {
		TeamName string `json:"team_name"`
	}
	expect(t, "aperçu", bob.do("POST", "/invitations/preview", map[string]string{"token": e.mailer.lastInvite}, &preview), 200)
	if preview.TeamName != "Acme" {
		t.Fatal("aperçu de l'invitation")
	}
	expect(t, "carol vole l'invitation", carol.do("POST", "/invitations/accept", map[string]string{"token": e.mailer.lastInvite}, nil), 403)
	expect(t, "bob accepte", bob.do("POST", "/invitations/accept", map[string]string{"token": e.mailer.lastInvite}, nil), 200)
	expect(t, "invitation consommée", bob.do("POST", "/invitations/accept", map[string]string{"token": e.mailer.lastInvite}, nil), 404)

	p := perms(t, bob, acme.ID)
	if !slices.Contains(p, "resource.deploy") || slices.Contains(p, "team.members.manage") {
		t.Fatalf("permissions développeur inattendues : %v", p)
	}
	expect(t, "bob ne peut pas inviter", bob.do("POST", "/teams/"+acme.ID+"/invitations",
		map[string]any{"email": "x@example.com", "role_id": roleID(t, bob, acme.ID, rbac.RoleViewer)}, nil), 403)
	expect(t, "bob ne peut pas créer de rôle", bob.do("POST", "/teams/"+acme.ID+"/roles",
		map[string]any{"name": "X", "permissions": []string{"team.read"}}, nil), 403)
	expect(t, "bob ne peut pas supprimer Acme", bob.do("DELETE", "/teams/"+acme.ID, nil, nil), 403)

	// Rôles personnalisés et anti-élévation de privilèges.
	expect(t, "permission inconnue", alice.do("POST", "/teams/"+acme.ID+"/roles",
		map[string]any{"name": "Bad", "permissions": []string{"server.everything"}}, nil), 422)
	expect(t, "rôle Délégué", alice.do("POST", "/teams/"+acme.ID+"/roles",
		map[string]any{"name": "Délégué", "permissions": []string{"team.read", "team.roles.manage", "team.members.manage"}}, nil), 201)
	expect(t, "rôle Ops", alice.do("POST", "/teams/"+acme.ID+"/roles",
		map[string]any{"name": "Ops", "permissions": []string{"server.*"}}, nil), 201)
	expect(t, "doublon de rôle", alice.do("POST", "/teams/"+acme.ID+"/roles",
		map[string]any{"name": "ops", "permissions": []string{"server.read"}}, nil), 409)
	expect(t, "attribution Délégué à bob", alice.do("POST", "/teams/"+acme.ID+"/role-assignments",
		map[string]any{"user_id": bobID, "role_id": roleID(t, alice, acme.ID, "Délégué")}, nil), 201)
	expect(t, "bob crée un rôle plus large que ses droits", bob.do("POST", "/teams/"+acme.ID+"/roles",
		map[string]any{"name": "Escalade", "permissions": []string{"server.delete"}}, nil), 403)
	expect(t, "bob s'attribue Ops", bob.do("POST", "/teams/"+acme.ID+"/role-assignments",
		map[string]any{"user_id": bobID, "role_id": roleID(t, alice, acme.ID, "Ops")}, nil), 403)
	expect(t, "bob s'attribue owner", bob.do("POST", "/teams/"+acme.ID+"/role-assignments",
		map[string]any{"user_id": bobID, "role_id": roleID(t, alice, acme.ID, rbac.RoleOwner)}, nil), 403)
	expect(t, "bob crée un rôle dans ses droits", bob.do("POST", "/teams/"+acme.ID+"/roles",
		map[string]any{"name": "Lecture", "permissions": []string{"team.read", "resource.read"}}, nil), 201)
	expect(t, "rôle système non modifiable", alice.do("PATCH", "/teams/"+acme.ID+"/roles/"+roleID(t, alice, acme.ID, rbac.RoleAdmin),
		map[string]any{"name": "X", "permissions": []string{"*"}}, nil), 403)
	expect(t, "membre inconnu", alice.do("POST", "/teams/"+acme.ID+"/role-assignments",
		map[string]any{"user_id": carolID, "role_id": roleID(t, alice, acme.ID, "Ops")}, nil), 422)

	// Deny : à portée égale, le refus l'emporte.
	expect(t, "rôle SansDéploiement", alice.do("POST", "/teams/"+acme.ID+"/roles",
		map[string]any{"name": "SansDéploiement", "permissions": []string{"resource.deploy"}}, nil), 201)
	var deny store.RoleAssignment
	expect(t, "deny déploiement bob", alice.do("POST", "/teams/"+acme.ID+"/role-assignments",
		map[string]any{"user_id": bobID, "role_id": roleID(t, alice, acme.ID, "SansDéploiement"), "effect": "deny"}, &deny), 201)
	if p := perms(t, bob, acme.ID); slices.Contains(p, "resource.deploy") || !slices.Contains(p, "resource.update") {
		t.Fatalf("le deny ne doit retirer que resource.deploy : %v", p)
	}
	expect(t, "déploiement refusé", bob.raw("POST", e.srv.URL+"/test/teams/"+acme.ID+"/deploy", nil, nil), 403)
	expect(t, "suppression du deny", alice.do("DELETE", "/teams/"+acme.ID+"/role-assignments/"+deny.ID, nil, nil), 204)
	if !slices.Contains(perms(t, bob, acme.ID), "resource.deploy") {
		t.Fatal("le déploiement doit être de nouveau autorisé")
	}

	// Groupes : les droits du groupe s'appliquent à ses membres ; anti-élévation via les groupes.
	var sre, admins store.Group
	expect(t, "groupe SRE", alice.do("POST", "/teams/"+acme.ID+"/groups", map[string]string{"name": "SRE"}, &sre), 201)
	expect(t, "groupe Admins", alice.do("POST", "/teams/"+acme.ID+"/groups", map[string]string{"name": "Admins"}, &admins), 201)
	expect(t, "Ops au groupe SRE", alice.do("POST", "/teams/"+acme.ID+"/role-assignments",
		map[string]any{"group_id": sre.ID, "role_id": roleID(t, alice, acme.ID, "Ops")}, nil), 201)
	expect(t, "admin au groupe Admins", alice.do("POST", "/teams/"+acme.ID+"/role-assignments",
		map[string]any{"group_id": admins.ID, "role_id": roleID(t, alice, acme.ID, rbac.RoleAdmin)}, nil), 201)
	expect(t, "owner à un groupe interdit", alice.do("POST", "/teams/"+acme.ID+"/role-assignments",
		map[string]any{"group_id": admins.ID, "role_id": roleID(t, alice, acme.ID, rbac.RoleOwner)}, nil), 422)
	expect(t, "bob s'ajoute aux Admins", bob.do("PUT", "/teams/"+acme.ID+"/groups/"+admins.ID+"/members/"+bobID, nil, nil), 403)
	expect(t, "carol non membre", alice.do("PUT", "/teams/"+acme.ID+"/groups/"+sre.ID+"/members/"+carolID, nil, nil), 422)
	expect(t, "bob dans SRE", alice.do("PUT", "/teams/"+acme.ID+"/groups/"+sre.ID+"/members/"+bobID, nil, nil), 204)
	if !slices.Contains(perms(t, bob, acme.ID), "server.delete") {
		t.Fatal("bob hérite des droits du groupe SRE")
	}
	var groups []store.Group
	expect(t, "liste groupes", alice.do("GET", "/teams/"+acme.ID+"/groups", nil, &groups), 200)
	if len(groups) != 2 || len(groups[1].Members) != 1 {
		t.Fatalf("groupes inattendus : %+v", groups)
	}
	expect(t, "bob retiré de SRE", alice.do("DELETE", "/teams/"+acme.ID+"/groups/"+sre.ID+"/members/"+bobID, nil, nil), 204)
	if slices.Contains(perms(t, bob, acme.ID), "server.delete") {
		t.Fatal("bob perd les droits du groupe")
	}

	// Sous-équipes : héritage des droits, membres directs limités à la sous-équipe.
	var front store.Team
	expect(t, "sous-équipe par bob refusée", bob.do("POST", "/teams", map[string]any{"name": "Front", "parent_id": acme.ID}, nil), 403)
	expect(t, "sous-équipe", alice.do("POST", "/teams", map[string]any{"name": "Front", "parent_id": acme.ID}, &front), 201)
	expect(t, "bob voit la sous-équipe", bob.do("GET", "/teams", nil, &teams), 200)
	if !slices.ContainsFunc(teams, func(t store.Team) bool { return t.ID == front.ID }) {
		t.Fatal("bob hérite de l'accès à la sous-équipe")
	}
	if !slices.Contains(perms(t, bob, front.ID), "resource.deploy") {
		t.Fatal("bob hérite du rôle développeur sur la sous-équipe")
	}
	invite(t, e, alice, carol, front.ID, "carol@example.com", rbac.RoleViewer)
	expect(t, "carol voit Front", carol.do("GET", "/teams/"+front.ID, nil, nil), 200)
	expect(t, "carol ne voit pas Acme", carol.do("GET", "/teams/"+acme.ID, nil, nil), 404)
	if p := perms(t, carol, front.ID); slices.Contains(p, "resource.deploy") {
		t.Fatalf("carol est lectrice : %v", p)
	}
	var members []store.Member
	expect(t, "membres Front", alice.do("GET", "/teams/"+front.ID+"/members", nil, &members), 200)
	if len(members) != 3 {
		t.Fatalf("3 membres attendus (2 hérités + carol), obtenu %d", len(members))
	}
	expect(t, "membre hérité non retirable ici", alice.do("DELETE", "/teams/"+front.ID+"/members/"+bobID, nil, nil), 409)

	// Propriété : dernier propriétaire protégé, transfert.
	expect(t, "alice ne peut pas quitter Acme", alice.do("POST", "/teams/"+acme.ID+"/leave", nil, nil), 409)
	expect(t, "transfert par non-owner", bob.do("POST", "/teams/"+front.ID+"/transfer-ownership", map[string]string{"user_id": carolID}, nil), 403)
	expect(t, "transfert Front à carol", alice.do("POST", "/teams/"+front.ID+"/transfer-ownership", map[string]string{"user_id": carolID}, nil), 204)
	expect(t, "Front carol", carol.do("GET", "/teams/"+front.ID, nil, &info), 200)
	if !info.IsOwner {
		t.Fatal("carol est propriétaire de Front")
	}

	// Jetons API rattachés à une équipe.
	var tok struct{ Token string }
	expect(t, "jeton bob Acme", bob.do("POST", "/me/tokens", map[string]any{"name": "ci", "team_id": acme.ID, "scopes": []string{"read"}}, &tok), 201)
	expect(t, "jeton pour équipe étrangère", carol.do("POST", "/me/tokens", map[string]any{"name": "x", "team_id": acme.ID}, nil), 422)
	api := newClient(t, e.srv)
	api.bearer = tok.Token
	expect(t, "équipes via jeton", api.do("GET", "/teams", nil, &teams), 200)
	if slices.ContainsFunc(teams, func(t store.Team) bool { return t.Personal }) || len(teams) != 2 {
		t.Fatalf("le jeton ne voit qu'Acme et ses sous-équipes : %+v", teams)
	}
	expect(t, "membres via jeton", api.do("GET", "/teams/"+acme.ID+"/members", nil, nil), 200)
	if p := perms(t, api, acme.ID); slices.Contains(p, "resource.update") || slices.Contains(p, "resource.env.read") {
		t.Fatalf("un jeton read est limité à la lecture : %v", p)
	}
	expect(t, "déploiement via jeton read", api.raw("POST", e.srv.URL+"/test/teams/"+acme.ID+"/deploy", nil, nil), 403)

	// Retrait d'un membre : perd l'accès à l'équipe et aux sous-équipes.
	expect(t, "bob retiré d'Acme", alice.do("DELETE", "/teams/"+acme.ID+"/members/"+bobID, nil, nil), 204)
	expect(t, "bob sans accès Acme", bob.do("GET", "/teams/"+acme.ID, nil, nil), 404)
	expect(t, "bob sans accès Front", bob.do("GET", "/teams/"+front.ID, nil, nil), 404)
	expect(t, "jeton de bob inutilisable", api.do("GET", "/teams/"+acme.ID+"/members", nil, nil), 404)

	// Audit
	var audit struct {
		Events []store.AuditEvent `json:"events"`
	}
	expect(t, "audit", alice.do("GET", "/teams/"+acme.ID+"/audit?limit=200", nil, &audit), 200)
	var actions []string
	for _, ev := range audit.Events {
		actions = append(actions, ev.Action)
	}
	for _, want := range []string{"team.created", "team.invitation.created", "team.invitation.accepted", "team.role.created",
		"team.role_assignment.created", "team.role_assignment.deleted", "team.group.member_added", "team.member.removed",
		"team.ownership.transferred", "auth.token.created"} {
		if !slices.Contains(actions, want) {
			t.Errorf("événement d'audit manquant : %s", want)
		}
	}
	expect(t, "audit filtré", alice.do("GET", "/teams/"+acme.ID+"/audit?action=team.invitation", nil, &audit), 200)
	for _, ev := range audit.Events {
		if !strings.HasPrefix(ev.Action, "team.invitation") {
			t.Fatalf("filtre action non respecté : %s", ev.Action)
		}
	}
	expect(t, "audit de Front par sa propriétaire", carol.do("GET", "/teams/"+front.ID+"/audit", nil, nil), 200)
	req, _ := http.NewRequest("GET", alice.base+"/teams/"+acme.ID+"/audit/export", nil)
	resp, err := alice.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/csv") || !strings.HasPrefix(string(body), "id,date,") {
		t.Fatalf("export CSV : %d %s", resp.StatusCode, body[:min(len(body), 80)])
	}
	if _, err := e.st.Pool.Exec(context.Background(), `UPDATE audit_events SET action = 'x'`); err == nil {
		t.Fatal("le journal d'audit doit être immuable")
	}

	// Suppression d'une sous-équipe : l'audit est conservé.
	expect(t, "suppression Front", alice.do("DELETE", "/teams/"+front.ID, nil, nil), 204)
	var n int
	_ = e.st.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events WHERE team_id = $1`, front.ID).Scan(&n)
	if n == 0 {
		t.Fatal("l'audit d'une équipe supprimée est conservé")
	}
}

func TestApprovals(t *testing.T) {
	e := newTestEnv(t)
	alice, _ := register(t, e, "alice@example.com")
	bob, _ := register(t, e, "bob@example.com")
	dave, _ := register(t, e, "dave@example.com")
	vic, _ := register(t, e, "vic@example.com")
	var acme store.Team
	expect(t, "Acme", alice.do("POST", "/teams", map[string]string{"name": "Acme"}, &acme), 201)
	invite(t, e, alice, bob, acme.ID, "bob@example.com", rbac.RoleDeveloper)
	invite(t, e, alice, dave, acme.ID, "dave@example.com", rbac.RoleAdmin)
	invite(t, e, alice, vic, acme.ID, "vic@example.com", rbac.RoleViewer)
	deploy := e.srv.URL + "/test/teams/" + acme.ID + "/deploy"

	expect(t, "déploiement sans règle", bob.raw("POST", deploy, nil, nil), 200)

	expect(t, "règle par développeur refusée", bob.do("POST", "/teams/"+acme.ID+"/approval-rules",
		map[string]any{"name": "Prod", "actions": []string{"resource.deploy"}}, nil), 403)
	expect(t, "action non soumissible", alice.do("POST", "/teams/"+acme.ID+"/approval-rules",
		map[string]any{"name": "Prod", "actions": []string{"team.read"}}, nil), 422)
	var rule store.ApprovalRule
	expect(t, "règle 2 approbations", alice.do("POST", "/teams/"+acme.ID+"/approval-rules",
		map[string]any{"name": "Prod", "actions": []string{"resource.deploy"}, "required_approvals": 2}, &rule), 201)

	var pending struct {
		ApprovalRequired bool                  `json:"approval_required"`
		ApprovalRequest  store.ApprovalRequest `json:"approval_request"`
	}
	expect(t, "déploiement soumis", bob.raw("POST", deploy, nil, &pending), 202)
	req := pending.ApprovalRequest
	if !pending.ApprovalRequired || req.Status != "pending" || req.RequiredApprovals != 2 {
		t.Fatalf("demande inattendue : %+v", pending)
	}
	path := "/teams/" + acme.ID + "/approvals/" + req.ID
	expect(t, "auto-approbation", bob.do("POST", path+"/approve", nil, nil), 403)
	expect(t, "lecteur sans droit d'approuver", vic.do("POST", path+"/approve", nil, nil), 403)
	var got store.ApprovalRequest
	expect(t, "approbation alice", alice.do("POST", path+"/approve", map[string]string{"comment": "ok"}, &got), 200)
	if got.Status != "pending" || len(e.executed) != 0 {
		t.Fatal("1/2 approbations : toujours en attente")
	}
	expect(t, "double vote", alice.do("POST", path+"/approve", nil, nil), 409)
	expect(t, "approbation dave", dave.do("POST", path+"/approve", nil, &got), 200)
	if got.Status != "executed" || len(e.executed) != 1 || e.executed[0].ID != req.ID {
		t.Fatalf("2/2 approbations : l'action doit être exécutée (statut %s)", got.Status)
	}
	expect(t, "vote après exécution", vic.do("POST", path+"/reject", nil, nil), 403)

	// Rejet
	expect(t, "deuxième demande", bob.raw("POST", deploy, nil, &pending), 202)
	expect(t, "rejet", alice.do("POST", "/teams/"+acme.ID+"/approvals/"+pending.ApprovalRequest.ID+"/reject",
		map[string]string{"comment": "pas maintenant"}, &got), 200)
	if got.Status != "rejected" || len(e.executed) != 1 {
		t.Fatal("une demande rejetée n'est pas exécutée")
	}
	// Annulation par le demandeur
	expect(t, "troisième demande", bob.raw("POST", deploy, nil, &pending), 202)
	expect(t, "annulation par un lecteur", vic.do("POST", "/teams/"+acme.ID+"/approvals/"+pending.ApprovalRequest.ID+"/cancel", nil, nil), 403)
	expect(t, "annulation", bob.do("POST", "/teams/"+acme.ID+"/approvals/"+pending.ApprovalRequest.ID+"/cancel", nil, nil), 204)

	var list []store.ApprovalRequest
	expect(t, "liste exécutées", alice.do("GET", "/teams/"+acme.ID+"/approvals?status=executed", nil, &list), 200)
	if len(list) != 1 {
		t.Fatalf("1 demande exécutée attendue, %d", len(list))
	}

	// Désactivation de la règle
	expect(t, "désactivation", alice.do("PATCH", "/teams/"+acme.ID+"/approval-rules/"+rule.ID,
		map[string]any{"name": "Prod", "actions": []string{"resource.deploy"}, "required_approvals": 2, "enabled": false}, nil), 204)
	expect(t, "déploiement direct", bob.raw("POST", deploy, nil, nil), 200)
}
