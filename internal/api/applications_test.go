package api

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/ssh/sshtest"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// appFixture : équipe, serveur validé (destination par défaut), projet et environnement.
type appFixture struct {
	e                       *testEnv
	alice                   *client
	team, base              string
	serverID, destID        string
	projectID, envID, keyID string
	ssh                     *sshtest.Server
}

func newAppFixture(t *testing.T) *appFixture {
	e := newTestEnv(t)
	f := &appFixture{e: e}
	f.alice, _ = register(t, e, "alice@example.com")
	var acme store.Team
	expect(t, "Acme", f.alice.do("POST", "/teams", map[string]string{"name": "Acme"}, &acme), 201)
	f.team, f.base = acme.ID, "/teams/"+acme.ID
	var key store.PrivateKey
	expect(t, "clé", f.alice.do("POST", f.base+"/private-keys", map[string]string{"name": "k"}, &key), 201)
	f.keyID = key.ID
	var dockerOK atomic.Bool
	dockerOK.Store(true)
	f.ssh = sshtest.New(t, key.PublicKey, fakeUbuntu(&dockerOK))
	var created struct {
		Server store.Server     `json:"server"`
		Task   store.ServerTask `json:"task"`
	}
	expect(t, "serveur", f.alice.do("POST", f.base+"/servers", map[string]any{"name": "s1", "ip": f.ssh.Host, "port": f.ssh.Port,
		"private_key_id": key.ID, "proxy_type": "none"}, &created), 201)
	f.serverID = created.Server.ID
	waitTask(t, f.alice, f.team, f.serverID, created.Task.ID)
	var dests []store.Destination
	expect(t, "destinations", f.alice.do("GET", f.base+"/servers/"+f.serverID+"/destinations", nil, &dests), 200)
	f.destID = dests[0].ID
	var p store.Project
	expect(t, "projet", f.alice.do("POST", f.base+"/projects", map[string]string{"name": "Web"}, &p), 201)
	var detail struct {
		Environments []store.Environment `json:"environments"`
	}
	expect(t, "env", f.alice.do("GET", f.base+"/projects/"+p.ID, nil, &detail), 200)
	f.projectID, f.envID = p.ID, detail.Environments[0].ID
	return f
}

func (f *appFixture) app(extra map[string]any) map[string]any {
	m := map[string]any{"environment_id": f.envID, "destination_id": f.destID, "name": "Site", "build_pack": "nixpacks",
		"git_repository": "https://github.com/acme/site"}
	for k, v := range extra {
		m[k] = v
	}
	return m
}

func TestApplications(t *testing.T) {
	f := newAppFixture(t)
	alice, base := f.alice, f.base
	bob, bobID := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)

	// Validation
	for name, body := range map[string]map[string]any{
		"build pack inconnu":          f.app(map[string]any{"build_pack": "heroku"}),
		"dépôt manquant":              f.app(map[string]any{"git_repository": ""}),
		"dépôt public non URL":        f.app(map[string]any{"git_repository": "acme/site"}),
		"source avec URL":             f.app(map[string]any{"source_id": f.keyID}),
		"deploy key avec https":       f.app(map[string]any{"private_key_id": f.keyID}),
		"domaine invalide":            f.app(map[string]any{"domains": []string{"https://pas un domaine"}}),
		"chemin avec ..":              f.app(map[string]any{"base_directory": "/../etc"}),
		"ports invalides":             f.app(map[string]any{"ports_exposes": "3000,abc"}),
		"branche invalide":            f.app(map[string]any{"git_branch": "-x"}),
		"limite mémoire invalide":     f.app(map[string]any{"limits": map[string]any{"memory": "beaucoup"}}),
		"compose YAML invalide":       f.app(map[string]any{"build_pack": "dockercompose", "git_repository": "", "docker_compose_raw": "services: ["}),
		"auth HTTP sans mot de passe": f.app(map[string]any{"settings": map[string]any{"basic_auth_enabled": true, "basic_auth_username": "u"}}),
	} {
		expect(t, name, alice.do("POST", base+"/applications", body, nil), 422)
	}
	expect(t, "lecteur sans resource.create", bob.do("POST", base+"/applications", f.app(nil), nil), 403)

	// Création : domaine sslip.io généré, port par défaut, réglages par défaut.
	var site map[string]any
	expect(t, "création", alice.do("POST", base+"/applications", f.app(nil), &site), 201)
	siteID := site["id"].(string)
	domains := site["domains"].([]any)
	if len(domains) != 1 || !strings.HasSuffix(domains[0].(string), ".127.0.0.1.sslip.io") || site["ports_exposes"] != "3000" {
		t.Fatalf("valeurs par défaut : %v %v", domains, site["ports_exposes"])
	}
	if site["settings"].(map[string]any)["auto_deploy"] != true || site["health_check"].(map[string]any)["path"] != "/" {
		t.Fatal("réglages par défaut")
	}

	// Conflit de domaine
	expect(t, "domaine explicite", alice.do("PATCH", base+"/applications/"+siteID, map[string]any{"domains": []string{"https://app.example.com", "www.app.example.com/"}}, &site), 200)
	if d := site["domains"].([]any); len(d) != 2 || d[1] != "https://www.app.example.com" {
		t.Fatalf("normalisation des domaines : %v", d)
	}
	expect(t, "conflit", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Autre", "domains": []string{"https://APP.example.com/api"}}), nil), 409)
	expect(t, "conflit forcé", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Autre", "domains": []string{"https://app.example.com/api"}, "force_domain_override": true}), nil), 201)

	// Autres build packs
	var img map[string]any
	expect(t, "image Docker", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Img", "build_pack": "dockerimage", "git_repository": "", "docker_image": "nginx", "docker_image_tag": "1.27"}), &img), 201)
	expect(t, "compose brut", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Stack", "build_pack": "dockercompose", "git_repository": "",
		"docker_compose_raw": "services:\n  web:\n    image: nginx\n", "docker_compose_domains": map[string]any{"web": []string{"stack.example.com"}}}), nil), 201)
	expect(t, "dockerfile en ligne", alice.do("POST", base+"/applications", f.app(map[string]any{"name": "Inline", "build_pack": "dockerfile", "git_repository": "", "dockerfile": "FROM nginx\n"}), nil), 201)

	// Mise à jour partielle des réglages ; hash de l'authentification HTTP jamais exposé ni injectable.
	expect(t, "réglages", alice.do("PATCH", base+"/applications/"+siteID, map[string]any{"settings": map[string]any{"force_https": false,
		"basic_auth_enabled": true, "basic_auth_username": "admin"}, "basic_auth_password": "secret"}, &site), 200)
	st := site["settings"].(map[string]any)
	if st["force_https"] != false || st["auto_deploy"] != true || st["basic_auth_password_hash"] != nil || site["basic_auth_password_set"] != true {
		t.Fatalf("réglages fusionnés : %v", st)
	}
	expect(t, "injection de hash", alice.do("PATCH", base+"/applications/"+siteID, map[string]any{"settings": map[string]any{"basic_auth_password_hash": "x"}}, nil), 200)
	stored, _ := f.e.st.ApplicationByIDAny(context.Background(), siteID)
	if !strings.HasPrefix(stored.Settings.BasicAuthPasswordHash, "$2a$") {
		t.Fatal("le hash bcrypt ne doit pas être remplaçable directement")
	}

	// Variables d'environnement
	envs := base + "/applications/" + siteID + "/envs"
	expect(t, "clé invalide", alice.do("POST", envs, map[string]any{"key": "1A", "value": "x"}, nil), 422)
	expect(t, "variable", alice.do("POST", envs, map[string]any{"key": "DATABASE_URL", "value": "postgres://{{team.DB}}"}, nil), 200)
	expect(t, "secret", alice.do("POST", envs, map[string]any{"key": "API_KEY", "value": "s3cr3t", "is_secret": true}, nil), 200)
	expect(t, "variable de preview", alice.do("POST", envs, map[string]any{"key": "DATABASE_URL", "value": "preview", "is_preview": true}, nil), 200)
	expect(t, "import .env", alice.do("POST", envs+"/bulk", map[string]any{"content": "PORT=3000\nNODE_ENV=production\nAPI_KEY=nouveau\n"}, nil), 200)
	var list []store.EnvVar
	expect(t, "liste", alice.do("GET", envs, nil, &list), 200)
	vals := map[string]store.EnvVar{}
	for _, v := range list {
		if !v.IsPreview {
			vals[v.Key] = v
		}
	}
	if len(list) != 5 || vals["DATABASE_URL"].Value != "postgres://{{team.DB}}" || vals["PORT"].Value != "3000" {
		t.Fatalf("variables : %+v", list)
	}
	if !vals["API_KEY"].IsSecret || vals["API_KEY"].Value != "" {
		t.Fatal("un secret reste secret, même réimporté")
	}
	expect(t, "lecteur sans env.read", bob.do("GET", envs, nil, nil), 403)
	expect(t, "ni build ni runtime", alice.do("PATCH", envs+"/"+vals["PORT"].ID, map[string]any{"is_buildtime": false, "is_runtime": false}, nil), 422)
	expect(t, "suppression variable", alice.do("DELETE", envs+"/"+vals["PORT"].ID, nil, nil), 200)

	// Écriture de variables soumise à approbation : valeurs chiffrées dans la demande.
	dave, _ := register(t, f.e, "dave@example.com")
	invite(t, f.e, alice, dave, f.team, "dave@example.com", rbac.RoleAdmin)
	expect(t, "règle", alice.do("POST", base+"/approval-rules", map[string]any{"name": "Vars", "actions": []string{"resource.env.write"},
		"scope": map[string]string{"type": "resource", "id": siteID}}, nil), 201)
	var pending struct {
		ApprovalRequest store.ApprovalRequest `json:"approval_request"`
	}
	expect(t, "variable soumise", alice.do("POST", envs, map[string]any{"key": "STRIPE_KEY", "value": "sk_live_tres_secret"}, &pending), 202)
	if strings.Contains(string(pending.ApprovalRequest.Payload), "sk_live") {
		t.Fatal("la valeur ne doit pas apparaître en clair dans la demande d'approbation")
	}
	var got store.ApprovalRequest
	expect(t, "approbation", dave.do("POST", base+"/approvals/"+pending.ApprovalRequest.ID+"/approve", nil, &got), 200)
	if got.Status != "executed" {
		t.Fatalf("exécution : %s %s", got.Status, got.Error)
	}
	expect(t, "liste", alice.do("GET", envs, nil, &list), 200)
	found := false
	for _, v := range list {
		found = found || (v.Key == "STRIPE_KEY" && v.Value == "sk_live_tres_secret")
	}
	if !found {
		t.Fatal("variable appliquée après approbation")
	}

	// Stockages
	stg := base + "/applications/" + siteID + "/storages"
	var vol, file store.Storage
	expect(t, "volume", alice.do("POST", stg, map[string]any{"kind": "volume", "mount_path": "/data"}, &vol), 201)
	if !strings.HasSuffix(vol.Name, "-data") {
		t.Fatalf("nom de volume par défaut : %s", vol.Name)
	}
	expect(t, "doublon de montage", alice.do("POST", stg, map[string]any{"kind": "volume", "mount_path": "/data/"}, nil), 409)
	expect(t, "bind invalide", alice.do("POST", stg, map[string]any{"kind": "bind", "host_path": "relatif", "mount_path": "/x"}, nil), 422)
	expect(t, "bind", alice.do("POST", stg, map[string]any{"kind": "bind", "host_path": "/srv/uploads", "mount_path": "/app/uploads"}, nil), 201)
	expect(t, "fichier", alice.do("POST", stg, map[string]any{"kind": "file", "mount_path": "/etc/nginx/conf.d/site.conf", "content": "server {}"}, &file), 201)
	expect(t, "contenu", alice.do("PATCH", stg+"/"+file.ID, map[string]string{"content": "server { listen 80; }"}, nil), 204)
	expect(t, "modif d'un volume", alice.do("PATCH", stg+"/"+vol.ID, map[string]string{"content": "x"}, nil), 404)
	expect(t, "suppression", alice.do("DELETE", stg+"/"+vol.ID, nil, nil), 204)

	// Tags, webhooks
	var tag store.Tag
	expect(t, "tag", alice.do("POST", base+"/tags", map[string]string{"name": "prod"}, &tag), 201)
	var tags []store.Tag
	expect(t, "tags", alice.do("PUT", base+"/applications/"+siteID+"/tags", map[string]any{"tag_ids": []string{tag.ID}}, &tags), 200)
	if len(tags) != 1 {
		t.Fatal("tag associé")
	}
	var hooks, rotated struct {
		Secret string
		URLs   map[string]string `json:"urls"`
	}
	expect(t, "webhooks", alice.do("GET", base+"/applications/"+siteID+"/webhooks", nil, &hooks), 200)
	if len(hooks.Secret) != 64 || !strings.HasSuffix(hooks.URLs["deploy"], "/webhooks/deploy/"+siteID) {
		t.Fatalf("webhooks : %+v", hooks)
	}
	expect(t, "rotation", alice.do("POST", base+"/applications/"+siteID+"/webhooks/rotate", nil, &rotated), 200)
	if rotated.Secret == hooks.Secret {
		t.Fatal("le secret doit changer")
	}

	// Droits au niveau de la ressource : bob développeur de cette seule application.
	expect(t, "développeur sur Site", alice.do("POST", base+"/role-assignments", map[string]any{"user_id": bobID,
		"role_id": roleID(t, alice, f.team, rbac.RoleDeveloper), "scope": map[string]string{"type": "resource", "id": siteID}}, nil), 201)
	expect(t, "bob modifie Site", bob.do("PATCH", base+"/applications/"+siteID, map[string]any{"description": "vitrine"}, nil), 200)
	expect(t, "bob ne modifie pas Img", bob.do("PATCH", base+"/applications/"+img["id"].(string), map[string]any{"description": "x"}, nil), 403)

	// Références : serveur, environnement, clé utilisés par des applications.
	expect(t, "serveur utilisé", alice.do("DELETE", base+"/servers/"+f.serverID, nil, nil), 409)
	expect(t, "environnement utilisé", alice.do("DELETE", base+"/projects/"+f.projectID+"/environments/"+f.envID, nil, nil), 409)
	expect(t, "projet utilisé", alice.do("DELETE", base+"/projects/"+f.projectID, nil, nil), 409)

	// Suppression
	expect(t, "suppression", alice.do("DELETE", base+"/applications/"+siteID, nil, nil), 204)
	expect(t, "supprimée", alice.do("GET", base+"/applications/"+siteID, nil, nil), 404)
	var apps []map[string]any
	expect(t, "liste", alice.do("GET", base+"/applications?environment_id="+f.envID, nil, &apps), 200)
	if len(apps) != 4 {
		t.Fatalf("4 applications restantes : %d", len(apps))
	}
}
