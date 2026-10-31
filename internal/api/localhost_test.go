package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestLocalhostAndInstanceUpdate(t *testing.T) {
	e := newTestEnv(t)
	priv, _, err := ssh.GenerateKey("nodra-localhost")
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(keyFile, []byte(priv), 0o600); err != nil {
		t.Fatal(err)
	}
	e.api.Localhost = LocalhostConfig{KeyFile: keyFile, Host: "127.0.0.1", Port: 1}

	// Le premier administrateur reçoit le serveur localhost dans son équipe personnelle.
	alice, _ := register(t, e, "alice@example.com")
	var teams []store.Team
	expect(t, "équipes", alice.do("GET", "/teams", nil, &teams), 200)
	base := "/teams/" + teams[0].ID
	var list []store.Server
	expect(t, "serveurs", alice.do("GET", base+"/servers", nil, &list), 200)
	if len(list) != 1 || list[0].Name != "localhost" || list[0].IP != "127.0.0.1" || list[0].User != "root" || !list[0].IsLocalhost {
		t.Fatalf("serveur localhost : %+v", list)
	}
	localhost := list[0]
	var tasks []store.ServerTask
	expect(t, "tâches", alice.do("GET", base+"/servers/"+localhost.ID+"/tasks", nil, &tasks), 200)
	if len(tasks) != 1 || tasks[0].Kind != "validate" {
		t.Fatalf("validation lancée : %+v", tasks)
	}
	register(t, e, "bob@example.com")
	if err := e.api.BootstrapLocalhost(context.Background()); err != nil {
		t.Fatal(err)
	}
	expect(t, "serveurs", alice.do("GET", base+"/servers", nil, &list), 200)
	if len(list) != 1 {
		t.Fatalf("créé une seule fois : %+v", list)
	}

	// Domaine de l'instance servi par le proxy du serveur localhost.
	expect(t, "domaine", alice.do("PATCH", "/admin/settings", map[string]any{"fqdn": "https://nodra.example.com"}, nil), 200)
	var configs []store.ProxyConfig
	expect(t, "configs", alice.do("GET", base+"/servers/"+localhost.ID+"/proxy/configs", nil, &configs), 200)
	if len(configs) != 1 || configs[0].Name != "nodra-instance.yaml" || !strings.Contains(configs[0].Content, "Host(`nodra.example.com`)") {
		t.Fatalf("config du domaine : %+v", configs)
	}
	expect(t, "domaine retiré", alice.do("PATCH", "/admin/settings", map[string]any{"fqdn": ""}, nil), 200)
	expect(t, "configs", alice.do("GET", base+"/servers/"+localhost.ID+"/proxy/configs", nil, &configs), 200)
	if len(configs) != 0 {
		t.Fatalf("config retirée : %+v", configs)
	}

	// Mise à jour manuelle : update.sh lancé en arrière-plan sur l'hôte.
	expect(t, "sans version connue", alice.do("POST", "/admin/update", nil, nil), 422)
	if err := e.st.SetLatestVersion(context.Background(), "v1.2.3"); err != nil {
		t.Fatal(err)
	}
	expect(t, "hors install.sh", alice.do("POST", "/admin/update", nil, nil), 409)
	e.api.InstallDir = "/data/nodra/source"
	expect(t, "version invalide", alice.do("POST", "/admin/update", map[string]string{"version": "v1; rm -rf /"}, nil), 422)
	var task store.ServerTask
	expect(t, "mise à jour", alice.do("POST", "/admin/update", nil, &task), 202)
	if task.Kind != "command" || task.ServerID != localhost.ID || !strings.Contains(string(task.Params), "update.sh 'v1.2.3'") ||
		!strings.Contains(string(task.Params), "nohup") {
		t.Fatalf("tâche de mise à jour : %+v %s", task, task.Params)
	}
	expect(t, "serveur supprimé", alice.do("DELETE", base+"/servers/"+localhost.ID, nil, nil), 204)
	expect(t, "plus de localhost", alice.do("POST", "/admin/update", nil, nil), 409)
	if err := e.api.BootstrapLocalhost(context.Background()); err != nil {
		t.Fatal(err)
	}
	expect(t, "serveurs", alice.do("GET", base+"/servers", nil, &list), 200)
	if len(list) != 0 {
		t.Fatalf("pas recréé après suppression : %+v", list)
	}
}
