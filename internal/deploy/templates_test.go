package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/services"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// TestTemplatesComposeConfig valide avec le vrai `docker compose config` le compose généré pour chaque
// modèle du catalogue (secrets et domaines compris). Lent : activé par NODRA_COMPOSE_CONFIG=1.
func TestTemplatesComposeConfig(t *testing.T) {
	if os.Getenv("NODRA_COMPOSE_CONFIG") == "" {
		t.Skip("NODRA_COMPOSE_CONFIG non défini")
	}
	dir := t.TempDir()
	failed := 0
	for _, tpl := range services.Catalog("", "") {
		full, _ := services.Get(tpl.Slug)
		an, err := services.Analyze(full.Compose)
		if err != nil {
			t.Errorf("%s : %v", tpl.Slug, err)
			continue
		}
		env := map[string]string{}
		for _, m := range an.SortedMagic() {
			if v, ok := services.Generate(m, env); ok {
				env[m.Name] = v
			}
		}
		for _, u := range an.User {
			if _, ok := env[u.Key]; !ok {
				env[u.Key] = u.Default
				if env[u.Key] == "" {
					env[u.Key] = "x" // une variable obligatoire doit être renseignée par l'utilisateur
				}
			}
		}
		var apps []*store.ServiceApp
		for _, name := range an.Services {
			a := &store.ServiceApp{Name: name}
			if m, ok := an.Exposed[name]; ok {
				a.URLVar, a.Port = m.ID, m.Port
				a.Domains = []string{"https://" + strings.ToLower(m.ID) + ".example.com"}
			}
			apps = append(apps, a)
		}
		for k, v := range ServiceURLVars(full.Compose, apps) {
			env[k] = v
		}
		out, files, err := ServiceCompose(ServiceSpec{Service: &store.Service{ID: "svc", TeamID: "t", DockerComposeRaw: full.Compose}, Apps: apps,
			Network: "nodra", ProxyType: "traefik", DataDir: dir + "/data"})
		if err != nil {
			t.Errorf("%s : %v", tpl.Slug, err)
			continue
		}
		for _, f := range files {
			if f.Dir {
				_ = os.MkdirAll(f.Path, 0o755)
			} else {
				_ = os.MkdirAll(filepath.Dir(f.Path), 0o755)
				_ = os.WriteFile(f.Path, []byte(InterpolateFile(f.Content, env)), 0o644)
			}
		}
		file, envFile := filepath.Join(dir, tpl.Slug+".yml"), filepath.Join(dir, tpl.Slug+".env")
		_ = os.WriteFile(file, []byte(out), 0o600)
		_ = os.WriteFile(envFile, []byte(dotenv(env, nil)), 0o600)
		cmd := exec.Command("docker", "compose", "-p", "svc", "-f", file, "--env-file", envFile, "config", "-q")
		cmd.Dir = dir
		if b, err := cmd.CombinedOutput(); err != nil {
			failed++
			t.Errorf("%s : %s", tpl.Slug, strings.TrimSpace(string(b)))
		}
	}
	t.Logf("%d modèles validés, %d en échec", len(services.Catalog("", ""))-failed, failed)
}
