package rbac

import "testing"

var (
	team   = Scope{ScopeTeam, "t"}
	parent = Scope{ScopeTeam, "p"}
	proj   = Scope{ScopeProject, "pr"}
	res    = Scope{ScopeResource, "r"}
)

func roles(key string) []string {
	for _, r := range SystemRoles() {
		if r.Key == key {
			return r.Permissions
		}
	}
	panic(key)
}

func TestDecide(t *testing.T) {
	chain := []Scope{res, proj, team, parent}
	dev := Assignment{Scope: team, Effect: Allow, RoleKey: RoleDeveloper, Permissions: roles(RoleDeveloper)}

	if !Decide(chain, []Assignment{dev}, "resource.deploy") {
		t.Fatal("un développeur d'équipe peut déployer")
	}
	if Decide(chain, []Assignment{dev}, "team.delete") {
		t.Fatal("un développeur ne peut pas supprimer l'équipe")
	}
	if Decide(chain, nil, "resource.read") {
		t.Fatal("sans attribution : refus")
	}

	// Deny plus précis l'emporte sur allow plus large.
	denyProj := Assignment{Scope: proj, Effect: Deny, Permissions: []string{"resource.deploy"}}
	if Decide(chain, []Assignment{dev, denyProj}, "resource.deploy") {
		t.Fatal("le deny sur le projet doit bloquer le déploiement")
	}
	if !Decide(chain, []Assignment{dev, denyProj}, "resource.read") {
		t.Fatal("le deny ne concerne que resource.deploy")
	}
	// Allow plus précis l'emporte sur deny plus large.
	denyTeam := Assignment{Scope: team, Effect: Deny, Permissions: []string{"resource.*"}}
	allowRes := Assignment{Scope: res, Effect: Allow, Permissions: []string{"resource.deploy"}}
	if !Decide(chain, []Assignment{denyTeam, allowRes}, "resource.deploy") {
		t.Fatal("l'allow sur la ressource doit l'emporter")
	}
	// À portée égale, deny l'emporte.
	if Decide(chain, []Assignment{dev, denyTeam}, "resource.read") {
		t.Fatal("à portée égale, deny l'emporte")
	}
	// Héritage depuis l'équipe parente.
	viewerParent := Assignment{Scope: parent, Effect: Allow, Permissions: roles(RoleViewer)}
	if !Decide(chain, []Assignment{viewerParent}, "resource.read") {
		t.Fatal("les droits de l'équipe parente sont hérités")
	}
	// Owner : tout, deny ignoré.
	owner := Assignment{Scope: parent, Effect: Allow, RoleKey: RoleOwner, Permissions: []string{"*"}}
	if !Decide(chain, []Assignment{owner, denyTeam}, "resource.deploy") || !IsOwner(chain, []Assignment{owner}) {
		t.Fatal("un propriétaire a tous les droits")
	}
	if IsOwner(chain, []Assignment{dev}) {
		t.Fatal("un développeur n'est pas propriétaire")
	}
	// Attribution hors chaîne ignorée.
	other := Assignment{Scope: Scope{ScopeProject, "autre"}, Effect: Allow, Permissions: []string{"*"}}
	if Decide(chain, []Assignment{other}, "resource.read") {
		t.Fatal("une attribution sur un autre projet ne s'applique pas")
	}
}

func TestGrants(t *testing.T) {
	if !Grants([]string{"resource.*"}, "resource.env.read") || Grants([]string{"resource.*"}, "server.read") {
		t.Fatal("joker de préfixe")
	}
	if !ValidGrant("server.*") || ValidGrant("nope.*") || ValidGrant("resource.deploi") || !ValidGrant("*") {
		t.Fatal("validation des permissions")
	}
	if len(Expand([]string{"*"})) != len(Catalog) {
		t.Fatal("* couvre tout le catalogue")
	}
	for _, r := range SystemRoles() {
		for _, p := range r.Permissions {
			if !ValidGrant(p) {
				t.Fatalf("rôle %s : permission inconnue %s", r.Key, p)
			}
		}
	}
}

func TestTokenAllows(t *testing.T) {
	cases := []struct {
		scopes []string
		perm   string
		want   bool
	}{
		{[]string{TokenRead}, "resource.read", true},
		{[]string{TokenRead}, "resource.env.read", false},
		{[]string{TokenRead}, "resource.update", false},
		{[]string{TokenWrite}, "resource.update", true},
		{[]string{TokenWrite}, "resource.deploy", false},
		{[]string{TokenWrite}, "team.members.manage", false},
		{[]string{TokenWrite}, "team.read", true},
		{[]string{TokenDeploy}, "resource.deploy", true},
		{[]string{TokenRoot}, "team.delete", true},
	}
	for _, c := range cases {
		if got := TokenAllows(c.scopes, c.perm); got != c.want {
			t.Errorf("%v %s : %v, attendu %v", c.scopes, c.perm, got, c.want)
		}
	}
}
