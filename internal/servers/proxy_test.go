package servers

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestDefaultProxyCompose(t *testing.T) {
	for _, typ := range []string{ProxyTraefik, ProxyCaddy} {
		c := DefaultProxyCompose(typ, "ops@example.com")
		if err := ValidateCompose(c); err != nil {
			t.Fatalf("%s : %v\n%s", typ, err, c)
		}
		var doc map[string]any
		_ = yaml.Unmarshal([]byte(c), &doc)
		if !strings.Contains(c, "ops@example.com") || doc["networks"] == nil {
			t.Fatalf("%s : e-mail ACME ou réseau manquant", typ)
		}
	}
	if DefaultProxyCompose(ProxyNone, "") != "" {
		t.Fatal("aucun compose sans proxy")
	}
	if ValidateCompose("services:\n  x:\n    image: nginx\n") == nil || ValidateCompose(":::") == nil {
		t.Fatal("compose invalide accepté")
	}
}

func TestValidateDynamicConfig(t *testing.T) {
	cases := []struct {
		typ, name, content string
		ok                 bool
	}{
		{ProxyTraefik, "redirect.yml", "http:\n  routers: {}\n", true},
		{ProxyTraefik, "redirect.yml", "http: [", false},
		{ProxyTraefik, "site.caddy", "x", false},
		{ProxyCaddy, "site.caddy", "example.com {\n  respond ok\n}\n", true},
		{ProxyCaddy, "site.yml", "a: b", false},
		{ProxyTraefik, "nodra-certs.yml", "{}", false},
	}
	for _, c := range cases {
		if err := ValidateDynamicConfig(c.typ, c.name, c.content); (err == nil) != c.ok {
			t.Errorf("%s %s : %v", c.typ, c.name, err)
		}
	}
	if ProxyConfigNameRe.MatchString("../etc/passwd.yml") || !ProxyConfigNameRe.MatchString("my_conf-1.yaml") {
		t.Fatal("validation des noms de fichiers")
	}
}

// Le script d'écriture est exécuté réellement (sans sudo) dans un dossier temporaire.
func TestWriteFilesScript(t *testing.T) {
	dir := t.TempDir()
	dyn := filepath.Join(dir, "dynamic")
	_ = os.MkdirAll(dyn, 0o755)
	_ = os.WriteFile(filepath.Join(dyn, "obsolete.yml"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dyn, "garde.yml"), []byte("ancien"), 0o644)
	files := map[string]string{
		filepath.Join(dir, "docker-compose.yml"): "services: {}\n",
		filepath.Join(dyn, "garde.yml"):          "nouveau 'contenu' $HOME\n",
		filepath.Join(dir, "certs", "a.key"):     "secret",
	}
	script := writeFilesScript(files, []string{dyn, filepath.Join(dir, "certs")})
	script = strings.ReplaceAll(script, "$SUDO ", "")
	if out, err := exec.Command("sh", "-c", script).CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	entries, _ := os.ReadDir(dyn)
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if strings.Join(names, ",") != "garde.yml" {
		t.Fatalf("fichiers restants : %v", names)
	}
	if b, _ := os.ReadFile(filepath.Join(dyn, "garde.yml")); string(b) != "nouveau 'contenu' $HOME\n" {
		t.Fatalf("contenu : %q", b)
	}
	if st, _ := os.Stat(filepath.Join(dir, "certs", "a.key")); st.Mode().Perm() != 0o600 {
		t.Fatalf("permissions de la clé : %v", st.Mode().Perm())
	}
}

func TestInstanceProxyConfig(t *testing.T) {
	name, cfg := InstanceProxyConfig(ProxyTraefik, "https://nodra.example.com")
	var parsed map[string]any
	if name != "nodra-instance.yaml" || yaml.Unmarshal([]byte(cfg), &parsed) != nil || parsed["http"] == nil ||
		!strings.Contains(cfg, "Host(`nodra.example.com`)") || !strings.Contains(cfg, "certResolver: letsencrypt") ||
		!strings.Contains(cfg, "redirectScheme") || !strings.Contains(cfg, "http://nodra:8000") {
		t.Fatalf("traefik :\n%s", cfg)
	}
	if _, cfg := InstanceProxyConfig(ProxyTraefik, "http://nodra.example.com"); strings.Contains(cfg, "tls") || strings.Contains(cfg, "redirect") {
		t.Fatalf("traefik http :\n%s", cfg)
	}
	name, cfg = InstanceProxyConfig(ProxyCaddy, "https://nodra.example.com")
	if name != "nodra-instance.caddy" || !strings.HasPrefix(cfg, "nodra.example.com {") {
		t.Fatalf("caddy :\n%s", cfg)
	}
	if name, _ := InstanceProxyConfig(ProxyNone, "https://nodra.example.com"); name != "" {
		t.Fatal("sans proxy : aucune configuration")
	}
}
