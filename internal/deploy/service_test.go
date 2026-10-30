package deploy

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

const svcCompose = `services:
  app:
    image: ghcr.io/x/app:1
    environment:
      - SERVICE_URL_APP_3000
      - DB=postgres://${SERVICE_USER_PG}:${SERVICE_PASSWORD_PG}@db/app
      - LINK=${SERVICE_FQDN_APP}
    volumes:
      - ./uploads:/data
      - app-cache:/cache
      - type: volume
        source: app-logs
        target: /logs
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: ${SERVICE_PASSWORD_PG}
      EMPTY:
  init:
    image: busybox
    command: echo ok
`

func TestServiceCompose(t *testing.T) {
	svc := &store.Service{ID: "s1", TeamID: "t1", DockerComposeRaw: svcCompose}
	apps := []*store.ServiceApp{
		{Name: "app", Domains: []string{"https://app.example.com:3000"}, URLVar: "APP"},
		{Name: "db"},
		{Name: "init", ExcludeFromStatus: true},
	}
	out, _, err := ServiceCompose(ServiceSpec{Service: svc, Apps: apps, Network: "nodra", ProxyType: "traefik", DataDir: "/data/nodra/services/s1/data"})
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
		Networks map[string]any            `yaml:"networks"`
		Volumes  map[string]any            `yaml:"volumes"`
	}
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Volumes["app-cache"] == nil || doc.Volumes["app-logs"] == nil || len(doc.Volumes) != 2 {
		t.Fatalf("volumes nommés déclarés (sans les montages de dossiers) : %v", doc.Volumes)
	}
	app := doc.Services["app"]
	env := app["environment"].(map[string]any)
	if env["SERVICE_URL_APP_3000"] != "${SERVICE_URL_APP_3000}" || env["DB"] != "postgres://${SERVICE_USER_PG}:${SERVICE_PASSWORD_PG}@db/app" {
		t.Fatalf("environnement : %v", env)
	}
	if doc.Services["db"]["environment"].(map[string]any)["EMPTY"] != "${EMPTY}" {
		t.Fatal("clé sans valeur rendue explicite")
	}
	labels := toStrings(app["labels"])
	if app["container_name"] != "app-s1" || !has(labels, "nodra.resource=s1") || !has(labels, "nodra.service=app") ||
		!has(labels, "traefik.http.services.https-0-s1-app.loadbalancer.server.port=3000") {
		t.Fatalf("labels app : %v", labels)
	}
	if !has(toStrings(doc.Services["init"]["labels"]), "nodra.status=ignore") {
		t.Fatal("init exclu du statut")
	}
	if slices.ContainsFunc(toStrings(doc.Services["db"]["labels"]), func(l string) bool { return strings.HasPrefix(l, "traefik.") }) {
		t.Fatal("pas de routage sans domaine")
	}
	if app["volumes"].([]any)[0] != "/data/nodra/services/s1/data/uploads:/data" || doc.Networks["nodra"] == nil {
		t.Fatalf("volumes / réseau : %v", app["volumes"])
	}

	vars := ServiceURLVars(svcCompose, apps)
	if vars["SERVICE_URL_APP_3000"] != "https://app.example.com" || vars["SERVICE_FQDN_APP"] != "app.example.com" {
		t.Fatalf("variables d'URL : %v", vars)
	}
	if got := ServiceURLVars(svcCompose, nil); got["SERVICE_URL_APP_3000"] != "" || len(sortedKeys(got)) != 2 {
		t.Fatalf("sans domaine : %v", got)
	}
}

func TestServiceComposeCoolifyExtensions(t *testing.T) {
	raw := `services:
  web:
    image: nginx
    network_mode: host
    volumes:
      - type: bind
        source: ./conf/nginx.conf
        target: /etc/nginx/nginx.conf
        content: |
          server_name ${DOMAIN:-localhost};
          set $$x 1;
      - type: bind
        source: ./cache
        target: /cache
        isDirectory: true
      - memos/:/var/opt/memos
    healthcheck:
      test: ["CMD", "node", "-e", "fetch(\x60http://127.0.0.1:${process.env.PORT||80}\x60)"]
  init:
    image: busybox
    exclude_from_hc: true
`
	out, files, err := ServiceCompose(ServiceSpec{Service: &store.Service{ID: "s1", DockerComposeRaw: raw}, Network: "nodra", DataDir: "/d"})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Path != "/d/conf/nginx.conf" || !files[1].Dir || files[1].Path != "/d/cache" {
		t.Fatalf("fichiers : %+v", files)
	}
	if got := InterpolateFile(files[0].Content, map[string]string{"DOMAIN": "x.com"}); got != "server_name x.com;\nset $x 1;\n" {
		t.Fatalf("interpolation : %q", got)
	}
	if strings.Contains(out, "content:") || strings.Contains(out, "isDirectory") || strings.Contains(out, "exclude_from_hc") {
		t.Fatalf("extensions retirées :\n%s", out)
	}
	if !strings.Contains(out, "memos:/var/opt/memos") || !strings.Contains(out, "$${process.env.PORT||80}") {
		t.Fatalf("volume normalisé / interpolation échappée :\n%s", out)
	}
	var doc struct {
		Services map[string]map[string]any `yaml:"services"`
	}
	_ = yaml.Unmarshal([]byte(out), &doc)
	if _, ok := doc.Services["web"]["networks"]; ok {
		t.Fatal("pas de réseau avec network_mode")
	}
	if !has(toStrings(doc.Services["init"]["labels"]), "nodra.status=ignore") {
		t.Fatal("exclude_from_hc → exclu du statut")
	}
	bad := "services:\n  web:\n    image: x\n    volumes:\n      - type: bind\n        source: /etc/cron.d/x\n        target: /x\n        content: hack\n"
	if _, _, err := ServiceCompose(ServiceSpec{Service: &store.Service{ID: "s", DockerComposeRaw: bad}, Network: "n", DataDir: "/d"}); err == nil {
		t.Fatal("une source absolue avec contenu doit être refusée")
	}
}
