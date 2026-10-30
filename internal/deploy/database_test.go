package deploy

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

func testDB(typ string) *store.Database {
	t := DBTypes[typ]
	return &store.Database{ID: "db1", TeamID: "t1", Type: typ, Name: "Main", Image: t.Image, Username: t.User, DatabaseName: t.DB,
		Limits: store.DefaultLimits(), Settings: store.DefaultDatabaseSettings(), InitScripts: []store.InitScript{}}
}

func dbService(t *testing.T, s DatabaseSpec) (map[string]any, map[string]any) {
	t.Helper()
	out, err := DatabaseCompose(s)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	return doc["services"].(map[string]any)["database"].(map[string]any), doc
}

func TestDatabaseCompose(t *testing.T) {
	db := testDB("postgresql")
	db.CustomConfig = "max_connections = 50"
	db.InitScripts = []store.InitScript{{Filename: "01-schema.sql", Content: "CREATE TABLE t (id int);"}}
	db.PortsMappings = "5433:5432"
	db.Limits.Memory = "512m"
	env := DatabaseEnv(db, "pa$$word", "", map[string]string{"POSTGRES_DB": "app", "TZ": "Europe/Paris"})
	if env["POSTGRES_PASSWORD"] != "pa$$word" || env["PGPASSWORD"] != "pa$$word" || env["POSTGRES_DB"] != "app" || env["TZ"] != "Europe/Paris" {
		t.Fatalf("variables : %v", env)
	}
	storages := DefaultDatabaseStorages("postgresql", "db1")
	if len(storages) != 1 || storages[0].Name != "postgresql-data-db1" || storages[0].MountPath != "/var/lib/postgresql/data" {
		t.Fatalf("volumes par défaut : %+v", storages[0])
	}
	svc, doc := dbService(t, DatabaseSpec{DB: db, Env: env, Network: "nodra", Storages: storages, Dir: "/data/nodra/databases/db1"})
	// Les valeurs sont protégées de l'interpolation de compose.
	if e := svc["environment"].(map[string]any); e["POSTGRES_PASSWORD"] != "pa$$$$word" {
		t.Fatalf("échappement : %v", e["POSTGRES_PASSWORD"])
	}
	if svc["container_name"] != "db1" || svc["image"] != "postgres:17-alpine" || svc["mem_limit"] != "512m" {
		t.Fatalf("service : %v", svc)
	}
	labels := strings.Join(toStrings(svc["labels"]), " ")
	for _, want := range []string{"nodra.managed=true", "nodra.resource=db1", "nodra.resource.type=database", "nodra.pr=0", "nodra.service=database"} {
		if !strings.Contains(labels, want) {
			t.Errorf("label manquant %s", want)
		}
	}
	if c := toStrings(svc["command"]); strings.Join(c, " ") != "postgres -c config_file=/etc/postgresql/postgresql.conf" {
		t.Fatalf("commande : %v", c)
	}
	hc := svc["healthcheck"].(map[string]any)
	if test := toStrings(hc["test"]); !strings.Contains(test[1], `psql -U "$${POSTGRES_USER}"`) {
		t.Fatalf("healthcheck : %v", test)
	}
	vols := svc["volumes"].([]any)
	if vols[0] != "postgresql-data-db1:/var/lib/postgresql/data" || len(vols) != 3 {
		t.Fatalf("volumes : %v", vols)
	}
	if v := vols[2].(map[string]any); v["source"] != "/data/nodra/databases/db1/initdb/01-schema.sql" ||
		v["target"] != "/docker-entrypoint-initdb.d/01-schema.sql" || v["read_only"] != true {
		t.Fatalf("script d'initialisation : %v", v)
	}
	if p := toStrings(svc["ports"]); len(p) != 1 || p[0] != "5433:5432" {
		t.Fatalf("ports : %v", p)
	}
	if _, ok := doc["volumes"].(map[string]any)["postgresql-data-db1"]; !ok {
		t.Fatal("volume nommé non déclaré")
	}

	// Redis : mot de passe dans la commande (échappé), vérification de santé désactivable.
	r := testDB("redis")
	r.Settings.HealthCheckEnabled = false
	svc, _ = dbService(t, DatabaseSpec{DB: r, Env: DatabaseEnv(r, "s3cr$t", "", nil), Network: "nodra"})
	if c := strings.Join(toStrings(svc["command"]), " "); c != "redis-server --requirepass s3cr$$t --appendonly yes" {
		t.Fatalf("commande redis : %s", c)
	}
	if _, ok := svc["healthcheck"]; ok {
		t.Fatal("healthcheck désactivé")
	}
	// Dragonfly : ulimits ; options docker run converties.
	df := testDB("dragonfly")
	df.CustomDockerRunOptions = "--cap-add SYS_ADMIN"
	svc, _ = dbService(t, DatabaseSpec{DB: df, Env: DatabaseEnv(df, "x", "", nil), Network: "nodra"})
	if svc["ulimits"] == nil || !strings.Contains(strings.Join(toStrings(svc["cap_add"]), ","), "SYS_ADMIN") {
		t.Fatalf("dragonfly : %v", svc)
	}
}

func TestConnectionURLAndProxy(t *testing.T) {
	db := testDB("mysql")
	db.Username, db.DatabaseName = "app", "shop"
	if u := ConnectionURL(db, "p@ss:/word", "db1", 3306); u != "mysql://app:p%40ss%3A%2Fword@db1:3306/shop" {
		t.Fatalf("URL mysql : %s", u)
	}
	for typ, want := range map[string]string{
		"postgresql": "postgres://postgres:pw@h:5432/postgres",
		"mongodb":    "mongodb://root:pw@h:27017/?directConnection=true",
		"redis":      "redis://default:pw@h:6379/0",
		"keydb":      "redis://:pw@h:6379/0",
		"clickhouse": "clickhouse://default:pw@h:9000/default",
	} {
		if u := ConnectionURL(testDB(typ), "pw", "h", DBTypes[typ].Port); u != want {
			t.Errorf("URL %s : %s", typ, u)
		}
	}

	if _, _, err := DatabaseProxy(db, "nodra", "/d"); err == nil {
		t.Fatal("port public requis")
	}
	port := 33060
	db.PublicPort = &port
	compose, nginx, err := DatabaseProxy(db, "nodra", "/d")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"listen 33060;", "set $upstream db1:3306;", "resolver 127.0.0.11"} {
		if !strings.Contains(nginx, want) {
			t.Errorf("nginx.conf : %q manquant\n%s", want, nginx)
		}
	}
	for _, want := range []string{"33060:33060", "container_name: db1-proxy", "nodra.service=proxy", "source: /d/proxy/nginx.conf"} {
		if !strings.Contains(compose, want) {
			t.Errorf("compose du proxy : %q manquant\n%s", want, compose)
		}
	}
}
