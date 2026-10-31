package deploy

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

func testApp() *store.Application {
	return &store.Application{ID: "app1", TeamID: "t1", Name: "Site", BuildPack: "nixpacks", PortsExposes: "3000", Redirect: "both",
		HealthCheck: store.DefaultHealthCheck(), Limits: store.DefaultLimits(), Settings: store.DefaultAppSettings(),
		DockerComposeDomains: map[string][]string{}}
}

func has(labels []string, l string) bool { return slices.Contains(labels, l) }

func TestTraefikLabels(t *testing.T) {
	a := testApp()
	a.Settings.BasicAuthEnabled, a.Settings.BasicAuthUsername, a.Settings.BasicAuthPasswordHash = true, "admin", "$2a$10$abc"
	a.Redirect = "www"
	labels, err := Labels(RouteFor(a, []string{"https://example.com", "http://api.example.com:8080/v1"}, "traefik", "nodra", "app1", nil))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"traefik.enable=true",
		"traefik.http.routers.https-0-app1.rule=Host(`example.com`)",
		"traefik.http.routers.https-0-app1.tls.certresolver=letsencrypt",
		"traefik.http.routers.https-0-app1.middlewares=gzip,to-www-app1,auth-app1",
		"traefik.http.routers.http-0-app1.middlewares=redirect-to-https",
		"traefik.http.services.https-0-app1.loadbalancer.server.port=3000",
		"traefik.http.middlewares.auth-app1.basicauth.users=admin:$2a$10$abc",
		// Domaine http avec port du conteneur et chemin.
		"traefik.http.routers.http-1-app1.rule=Host(`api.example.com`) && PathPrefix(`/v1`)",
		"traefik.http.services.http-1-app1.loadbalancer.server.port=8080",
		"traefik.http.middlewares.strip-1-app1.stripprefix.prefixes=/v1",
		"traefik.http.routers.http-1-app1.middlewares=strip-1-app1,gzip,to-www-app1,auth-app1",
	} {
		if !has(labels, want) {
			t.Errorf("label manquant : %s", want)
		}
	}
	if slices.ContainsFunc(labels, func(l string) bool { return strings.HasPrefix(l, "traefik.http.routers.https-1-") }) {
		t.Error("pas de routeur https pour un domaine http")
	}
	a.Settings.ForceHTTPS = false
	labels, _ = Labels(RouteFor(a, []string{"https://example.com"}, "traefik", "nodra", "app1", nil))
	if !has(labels, "traefik.http.routers.http-0-app1.service=https-0-app1") {
		t.Error("sans HTTPS forcé, le routeur http sert aussi l'application")
	}
	if l, _ := Labels(RouteFor(a, nil, "traefik", "nodra", "app1", nil)); l != nil {
		t.Error("aucun label sans domaine")
	}
}

func TestCaddyLabels(t *testing.T) {
	a := testApp()
	a.Redirect = "non-www"
	labels, err := Labels(RouteFor(a, []string{"https://www.example.com", "https://example.com/api"}, "caddy", "nodra", "app1",
		map[string][2]string{"example.com": {"/certs/c.crt", "/certs/c.key"}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"caddy_ingress_network=nodra",
		"caddy_0=https://www.example.com",
		"caddy_0.handle=*",
		"caddy_0.handle.0_reverse_proxy={{upstreams 3000}}",
		"caddy_0.redir=https://example.com{uri}",
		"caddy_1.handle_path=/api*",
		"caddy_1.tls=/certs/c.crt /certs/c.key",
		"caddy_1.encode=zstd gzip",
	} {
		if !has(labels, want) {
			t.Errorf("label manquant : %s\n%v", want, labels)
		}
	}
}

func TestAppCompose(t *testing.T) {
	a := testApp()
	a.HealthCheck.Enabled = true
	a.Limits.Memory, a.Limits.CPUs = "512m", "1.5"
	a.PortsMappings = "8080:3000"
	a.CustomLabels = "traefik.http.routers.https-0-app1.rule=Host(`custom.com`)\n# commentaire\nmy.label=1"
	a.CustomDockerRunOptions = "--cap-add SYS_ADMIN --ulimit nofile=1024:2048 --init --shm-size=1g"
	a.Settings.BasicAuthEnabled, a.Settings.BasicAuthUsername, a.Settings.BasicAuthPasswordHash = true, "u", "$2a$x"
	out, err := AppCompose(AppSpec{App: a, DeploymentID: "dep1", ContainerName: "app1-abc", Image: "nodra/app1:sha", Network: "nodra",
		ProxyType: "traefik", Domains: []string{"https://example.com"}, Env: map[string]string{"A": "x$y", "B": "$HOME"},
		Literal: map[string]bool{"A": true}, Storages: []*store.Storage{{Kind: "volume", Name: "data", MountPath: "/data"},
			{ID: "f1", Kind: "file", MountPath: "/etc/app.conf"}}}, "/data/nodra/applications/app1/files")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
		Networks map[string]map[string]any `yaml:"networks"`
		Volumes  map[string]map[string]any `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("YAML invalide : %v\n%s", err, out)
	}
	svc := doc.Services["app"]
	labels := toStrings(svc["labels"])
	if svc["container_name"] != "app1-abc" || svc["image"] != "nodra/app1:sha" || svc["mem_limit"] != "512m" || svc["cpus"] != "1.5" {
		t.Fatalf("service : %v", svc)
	}
	env := svc["environment"].(map[string]any)
	if env["A"] != "x$$y" || env["B"] != "$HOME" {
		t.Fatalf("échappement des variables littérales : %v", env)
	}
	if !has(labels, "traefik.http.routers.https-0-app1.rule=Host(`custom.com`)") || !has(labels, "my.label=1") ||
		!has(labels, "nodra.resource=app1") || !has(labels, "traefik.http.middlewares.auth-app1.basicauth.users=u:$$2a$$x") {
		t.Fatalf("labels : %v", labels)
	}
	if !slices.Contains(toStrings(svc["volumes"]), "/data/nodra/applications/app1/files/f1:/etc/app.conf") || doc.Volumes["data"] == nil {
		t.Fatalf("volumes : %v", svc["volumes"])
	}
	if doc.Networks["nodra"]["external"] != true || svc["init"] != true || svc["shm_size"] != "1g" {
		t.Fatalf("réseau/options : %v", svc)
	}
	if hc := svc["healthcheck"].(map[string]any); !strings.Contains(toStrings(hc["test"])[1], "http://localhost:3000/") {
		t.Fatalf("healthcheck : %v", hc)
	}
	if !slices.Contains(toStrings(svc["ports"]), "8080:3000") {
		t.Fatal("ports publiés")
	}
	a.CustomDockerRunOptions = "--net host"
	if _, err := AppCompose(AppSpec{App: a, Network: "nodra"}, ""); err == nil {
		t.Fatal("option docker run non prise en charge acceptée")
	}
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func TestTransformCompose(t *testing.T) {
	a := testApp()
	a.BuildPack = "dockercompose"
	a.DockerComposeDomains = map[string][]string{"web": {"https://stack.example.com"}}
	raw := `services:
  web:
    image: nginx
    expose: ["8080"]
    labels:
      existing: "1"
  db:
    image: postgres
    restart: always
    networks: [backend]
    volumes: ["./pgdata:/var/lib/postgresql/data", "named:/x"]
networks:
  backend: {}
`
	out, err := TransformCompose(raw, a, "dep1", "nodra", "traefik", "/data/nodra/applications/app1/data", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
		Networks map[string]any            `yaml:"networks"`
	}
	_ = yaml.Unmarshal([]byte(out), &doc)
	web, db := doc.Services["web"], doc.Services["db"]
	wl := toStrings(web["labels"])
	if !has(wl, "existing=1") || !has(wl, "nodra.service=web") || !has(wl, "traefik.http.services.https-0-app1-web.loadbalancer.server.port=8080") {
		t.Fatalf("labels web : %v", wl)
	}
	if slices.ContainsFunc(toStrings(db["labels"]), func(l string) bool { return strings.HasPrefix(l, "traefik.") }) {
		t.Fatal("pas de routage pour un service sans domaine")
	}
	if db["restart"] != "always" || web["restart"] != "unless-stopped" || !slices.Contains(toStrings(db["networks"]), "nodra") {
		t.Fatalf("services : %v", doc.Services)
	}
	if doc.Networks["backend"] == nil || doc.Networks["nodra"] == nil {
		t.Fatal("réseaux conservés et ajoutés")
	}
	if vols := toStrings(db["volumes"]); vols[0] != "/data/nodra/applications/app1/data/pgdata:/var/lib/postgresql/data" || vols[1] != "named:/x" {
		t.Fatalf("montages relatifs redirigés : %v", vols)
	}
	if _, err := TransformCompose("services: [", a, "d", "nodra", "traefik", "", nil, 0); err == nil {
		t.Fatal("compose invalide accepté")
	}
}

func TestBuildEnv(t *testing.T) {
	vars := []Var{
		{Key: "DB", Value: "postgres://{{team.HOST}}/app", Runtime: true, Buildtime: true},
		{Key: "DB", Value: "postgres://preview", Runtime: true, Preview: true},
		{Key: "RAW", Value: "{{team.HOST}}$x", Runtime: true, Literal: true},
		{Key: "TOKEN", Value: "tok-secret", Runtime: true, Secret: true},
		{Key: "BAD", Value: "{{project.NOPE}}", Runtime: true},
	}
	d := Defaults{ResourceID: "app1", ContainerName: "c1", Branch: "main", Commit: "abc", Port: "3000",
		Domains: []string{"https://example.com", "https://www.example.com/x"}, Nixpacks: true, IncludeInBuild: true}
	e := BuildEnv(vars, map[string]string{"team.HOST": "db.internal"}, false, d)
	if e.Runtime["DB"] != "postgres://db.internal/app" || e.Build["DB"] != "postgres://db.internal/app" || e.Runtime["RAW"] != "{{team.HOST}}$x" || !e.Literal["RAW"] {
		t.Fatalf("résolution : %v", e.Runtime)
	}
	if e.Runtime["NODRA_FQDN"] != "example.com,www.example.com" || e.Runtime["PORT"] != "3000" || e.Runtime["HOST"] != "0.0.0.0" {
		t.Fatalf("variables par défaut : %v", e.Runtime)
	}
	if _, ok := e.Build["SOURCE_COMMIT"]; ok {
		t.Fatal("SOURCE_COMMIT exclu du build par défaut (cache)")
	}
	if _, ok := e.Build["NODRA_CONTAINER_NAME"]; ok || e.Runtime["NODRA_CONTAINER_NAME"] != "c1" {
		t.Fatal("NODRA_CONTAINER_NAME uniquement à l'exécution")
	}
	if len(e.Missing) != 1 || e.Missing[0] != "project.NOPE" || !slices.Contains(e.Secrets, "tok-secret") {
		t.Fatalf("manquantes/secrets : %v %v", e.Missing, e.Secrets)
	}
	p := BuildEnv(vars, nil, true, d)
	if p.Runtime["DB"] != "postgres://preview" {
		t.Fatal("la valeur de preview l'emporte en PR")
	}
	if Redact("token=tok-secret ok", e.Secrets) != "token=******** ok" {
		t.Fatal("masquage des secrets")
	}
}

func TestSplitArgs(t *testing.T) {
	got, err := splitArgs(`--add-host "a b:1.2.3.4" --label='x y'`)
	if err != nil || len(got) != 3 || got[1] != "a b:1.2.3.4" || got[2] != "--label=x y" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestTransformComposePreview(t *testing.T) {
	a := &store.Application{ID: "app1", TeamID: "t1", DockerComposeDomains: map[string][]string{"web": {"https://5.web.example.com"}},
		Settings: store.DefaultAppSettings()}
	raw := "services:\n  web:\n    image: nginx\n    container_name: site\n    ports: [\"80\"]\nvolumes:\n  data:\n    name: site-data\n"
	out, err := TransformCompose(raw, a, "dep1", "nodra", "traefik", "/data/app1/pr-5/data", nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Host(`5.web.example.com`)", "traefik.http.routers.https-0-app1-pr-5-web", "nodra.pr=5",
		"container_name: site-pr-5", "name: site-data-pr-5"} {
		if !strings.Contains(out, want) {
			t.Errorf("%q absent :\n%s", want, out)
		}
	}
	prod, _ := TransformCompose(raw, a, "dep1", "nodra", "traefik", "/data/app1/data", nil, 0)
	if strings.Contains(prod, "-pr-") || !strings.Contains(prod, "container_name: site\n") {
		t.Errorf("production :\n%s", prod)
	}
}
