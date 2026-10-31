package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Murathan-Aydin/nodra/internal/envvars"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// DatabasesDir : dossier des bases de données sur les serveurs.
const DatabasesDir = "/data/nodra/databases"

// Actions sur les bases, exécutées en tâches serveur (params : DatabaseParams).
const (
	TaskDatabaseStart   = "database_start"
	TaskDatabaseStop    = "database_stop"
	TaskDatabaseRestart = "database_restart"
	TaskDatabaseProxy   = "database_proxy" // démarre ou arrête le proxy TCP public selon is_public
)

type DatabaseParams struct {
	DatabaseID string `json:"database_id"`
}

// DBType : caractéristiques d'un type de base (alignées sur Coolify).
type DBType struct {
	Image      string            // image par défaut
	Port       int               // port interne
	Volumes    map[string]string // volumes de données créés à la création : suffixe → chemin dans le conteneur
	ConfigPath string            // configuration personnalisée ("" : non prise en charge)
	InitDir    string            // scripts d'initialisation ("" : non pris en charge)
	User       string            // utilisateur par défaut ("" : pas d'utilisateur configurable)
	DB         string            // base par défaut ("" : pas de base configurable)
	Root       bool              // mot de passe root distinct (MySQL, MariaDB)
}

var DBTypes = map[string]DBType{
	"postgresql": {Image: "postgres:17-alpine", Port: 5432, Volumes: map[string]string{"data": "/var/lib/postgresql/data"},
		ConfigPath: "/etc/postgresql/postgresql.conf", InitDir: "/docker-entrypoint-initdb.d", User: "postgres", DB: "postgres"},
	"mysql": {Image: "mysql:8", Port: 3306, Volumes: map[string]string{"data": "/var/lib/mysql"},
		ConfigPath: "/etc/mysql/conf.d/custom-config.cnf", InitDir: "/docker-entrypoint-initdb.d", User: "mysql", DB: "default", Root: true},
	"mariadb": {Image: "mariadb:11", Port: 3306, Volumes: map[string]string{"data": "/var/lib/mysql"},
		ConfigPath: "/etc/mysql/conf.d/custom-config.cnf", InitDir: "/docker-entrypoint-initdb.d", User: "mariadb", DB: "default", Root: true},
	"mongodb": {Image: "mongo:7", Port: 27017, Volumes: map[string]string{"db": "/data/db", "configdb": "/data/configdb"},
		ConfigPath: "/etc/mongo/mongod.conf", InitDir: "/docker-entrypoint-initdb.d", User: "root", DB: "default"},
	"redis":      {Image: "redis:7.2", Port: 6379, Volumes: map[string]string{"data": "/data"}, ConfigPath: "/usr/local/etc/redis/redis.conf"},
	"keydb":      {Image: "eqalpha/keydb:latest", Port: 6379, Volumes: map[string]string{"data": "/data"}, ConfigPath: "/etc/keydb/keydb.conf"},
	"dragonfly":  {Image: "docker.dragonflydb.io/dragonflydb/dragonfly", Port: 6379, Volumes: map[string]string{"data": "/data"}},
	"clickhouse": {Image: "clickhouse/clickhouse-server:25.8", Port: 9000, Volumes: map[string]string{"data": "/var/lib/clickhouse"}, User: "default", DB: "default"},
}

// DefaultDatabaseStorages : volumes de données d'une nouvelle base.
func DefaultDatabaseStorages(typ, id string) []*store.Storage {
	t := DBTypes[typ]
	keys := make([]string, 0, len(t.Volumes))
	for k := range t.Volumes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]*store.Storage, 0, len(keys))
	for _, k := range keys {
		out = append(out, &store.Storage{Kind: "volume", Name: typ + "-" + k + "-" + id, MountPath: t.Volumes[k]})
	}
	return out
}

// ConnectionURL : URL de connexion à la base pour un hôte et un port donnés (interne ou public).
func ConnectionURL(db *store.Database, password, host string, port int) string {
	u := url.URL{Host: host + ":" + strconv.Itoa(port), User: url.UserPassword(db.Username, password)}
	switch db.Type {
	case "postgresql":
		u.Scheme, u.Path = "postgres", "/"+db.DatabaseName
	case "mysql", "mariadb":
		u.Scheme, u.Path = "mysql", "/"+db.DatabaseName
	case "mongodb":
		u.Scheme, u.Path, u.RawQuery = "mongodb", "/", "directConnection=true"
	case "redis":
		u.Scheme, u.Path, u.User = "redis", "/0", url.UserPassword("default", password)
	case "keydb", "dragonfly":
		u.Scheme, u.Path, u.User = "redis", "/0", url.UserPassword("", password)
	case "clickhouse":
		u.Scheme, u.Path = "clickhouse", "/"+db.DatabaseName
	}
	if sslEnabled(db) {
		switch db.Type {
		case "postgresql":
			u.RawQuery = "sslmode=" + SSLMode(db)
		case "mysql", "mariadb":
			u.RawQuery = "ssl-mode=REQUIRED"
		case "mongodb":
			u.RawQuery += "&tls=true"
		}
	}
	return u.String()
}

// DatabaseEnv : variables du conteneur (identifiants attendus par l'image), complétées puis remplacées
// par les variables définies par l'utilisateur.
func DatabaseEnv(db *store.Database, password, rootPassword string, user map[string]string) map[string]string {
	env := map[string]string{"NODRA_RESOURCE_UUID": db.ID, "NODRA_CONTAINER_NAME": db.ID}
	set := func(kv ...string) {
		for i := 0; i+1 < len(kv); i += 2 {
			env[kv[i]] = kv[i+1]
		}
	}
	switch db.Type {
	case "postgresql":
		set("POSTGRES_USER", db.Username, "POSTGRES_PASSWORD", password, "POSTGRES_DB", db.DatabaseName,
			"PGUSER", db.Username, "PGPASSWORD", password)
		if db.Settings.PostgresInitdbArgs != "" {
			set("POSTGRES_INITDB_ARGS", db.Settings.PostgresInitdbArgs)
		}
		if db.Settings.PostgresHostAuthMethod != "" {
			set("POSTGRES_HOST_AUTH_METHOD", db.Settings.PostgresHostAuthMethod)
		}
	case "mysql":
		set("MYSQL_ROOT_PASSWORD", rootPassword, "MYSQL_DATABASE", db.DatabaseName, "MYSQL_USER", db.Username, "MYSQL_PASSWORD", password)
	case "mariadb":
		set("MARIADB_ROOT_PASSWORD", rootPassword, "MARIADB_DATABASE", db.DatabaseName, "MARIADB_USER", db.Username,
			"MARIADB_PASSWORD", password)
	case "mongodb":
		set("MONGO_INITDB_ROOT_USERNAME", db.Username, "MONGO_INITDB_ROOT_PASSWORD", password, "MONGO_INITDB_DATABASE", db.DatabaseName)
	case "redis":
		set("REDIS_PASSWORD", password, "REDIS_USERNAME", "default")
	case "keydb", "dragonfly":
		set("REDIS_PASSWORD", password)
	case "clickhouse":
		set("CLICKHOUSE_USER", db.Username, "CLICKHOUSE_PASSWORD", password, "CLICKHOUSE_DB", db.DatabaseName,
			"CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT", "1")
	}
	for k, v := range user {
		env[k] = v
	}
	return env
}

// DatabaseSpec : tout ce qu'il faut pour générer le compose d'une base.
type DatabaseSpec struct {
	DB       *store.Database
	Env      map[string]string // variables finales (DatabaseEnv), jamais interpolées par compose
	Network  string
	Storages []*store.Storage
	Dir      string // dossier de la base sur le serveur (fichiers, configuration, scripts d'initialisation)
}

func databaseLabels(db *store.Database, service string) []string {
	return []string{ManagedLabel, "nodra.resource=" + db.ID, "nodra.resource.type=database", "nodra.team=" + db.TeamID,
		"nodra.pr=0", "nodra.service=" + service}
}

// dbCommand renvoie la commande et la vérification de santé propres au type. Les vérifications lisent
// les identifiants dans l'environnement du conteneur ($$ : non interpolé par compose).
func dbCommand(db *store.Database, env map[string]string) (command []string, health []string, extra map[string]any) {
	conf := db.CustomConfig != "" && DBTypes[db.Type].ConfigPath != ""
	pass := envvars.EscapeCompose(env["REDIS_PASSWORD"])
	switch db.Type {
	case "postgresql":
		if conf {
			command = []string{"postgres", "-c", "config_file=/etc/postgresql/postgresql.conf"}
		}
		health = []string{"CMD-SHELL", `psql -U "$${POSTGRES_USER}" -d "$${POSTGRES_DB}" -c 'SELECT 1' > /dev/null`}
		if sslEnabled(db) {
			if len(command) == 0 {
				command = []string{"postgres"}
			}
			d := SSLTypes["postgresql"].Dir
			command = append(command, "-c", "ssl=on", "-c", "ssl_cert_file="+d+"/server.crt", "-c", "ssl_key_file="+d+"/server.key",
				"-c", "ssl_ca_file="+d+"/ca.crt")
		}
	case "mysql":
		health = []string{"CMD-SHELL", `mysqladmin ping -h 127.0.0.1 -u root -p"$${MYSQL_ROOT_PASSWORD}"`}
	case "mariadb":
		health = []string{"CMD", "healthcheck.sh", "--connect", "--innodb_initialized"}
	}
	if sslEnabled(db) && (db.Type == "mysql" || db.Type == "mariadb") {
		d := SSLTypes[db.Type].Dir
		command = append(command, "--ssl-cert="+d+"/server.crt", "--ssl-key="+d+"/server.key", "--ssl-ca="+d+"/ca.crt",
			"--require-secure-transport=1")
	}
	switch db.Type {
	case "mongodb":
		if conf {
			command = []string{"mongod", "--config", "/etc/mongo/mongod.conf"}
		}
		ping := `--quiet --eval "db.adminCommand('ping')" > /dev/null`
		if sslEnabled(db) {
			d := SSLTypes["mongodb"].Dir
			if len(command) == 0 {
				command = []string{"mongod"}
			}
			command = append(command, "--tlsMode="+SSLMode(db), "--tlsCertificateKeyFile="+d+"/server.pem", "--tlsCAFile="+d+"/ca.pem",
				"--tlsAllowConnectionsWithoutCertificates")
			if SSLMode(db) == "requireTLS" {
				ping = "--tls --tlsCAFile " + d + "/ca.pem " + ping
			}
		}
		health = []string{"CMD-SHELL", "if command -v mongosh > /dev/null 2>&1; then mongosh " + ping + "; else mongo " + ping + "; fi"}
	case "redis":
		command = []string{"redis-server"}
		if conf {
			command = append(command, "/usr/local/etc/redis/redis.conf")
		}
		command = append(command, "--requirepass", pass, "--appendonly", "yes")
		command = append(command, redisTLS(db)...)
		health = []string{"CMD-SHELL", `redis-cli -a "$${REDIS_PASSWORD}" --no-auth-warning ping | grep -q PONG`}
	case "keydb":
		command = []string{"keydb-server"}
		if conf {
			command = append(command, "/etc/keydb/keydb.conf")
		}
		command = append(command, "--requirepass", pass, "--appendonly", "yes")
		command = append(command, redisTLS(db)...)
		health = []string{"CMD-SHELL", `keydb-cli --pass "$${REDIS_PASSWORD}" ping | grep -q PONG`}
	case "dragonfly":
		command = []string{"dragonfly", "--requirepass", pass}
		health = []string{"CMD-SHELL", `redis-cli -a "$${REDIS_PASSWORD}" --no-auth-warning ping | grep -q PONG`}
		extra = map[string]any{"ulimits": map[string]any{"memlock": -1}}
	case "clickhouse":
		health = []string{"CMD-SHELL", `clickhouse-client --user "$${CLICKHOUSE_USER}" --password "$${CLICKHOUSE_PASSWORD}" --query 'SELECT 1' > /dev/null`}
		extra = map[string]any{"ulimits": map[string]any{"nofile": map[string]any{"soft": 262144, "hard": 262144}}}
	}
	return command, health, extra
}

// redisTLS : port TLS 6380 en plus du port 6379 (Redis, KeyDB).
func redisTLS(db *store.Database) []string {
	if !sslEnabled(db) {
		return nil
	}
	d := SSLTypes[db.Type].Dir
	return []string{"--tls-port", "6380", "--tls-cert-file", d + "/server.crt", "--tls-key-file", d + "/server.key",
		"--tls-ca-cert-file", d + "/ca.crt", "--tls-auth-clients", "optional"}
}

// DatabaseCompose génère le docker-compose d'une base (conteneur nommé d'après son identifiant).
func DatabaseCompose(s DatabaseSpec) (string, error) {
	db, t := s.DB, DBTypes[s.DB.Type]
	env := make(map[string]string, len(s.Env))
	for k, v := range s.Env {
		env[k] = envvars.EscapeCompose(v)
	}
	svc := map[string]any{
		"image": db.Image, "container_name": db.ID, "restart": "unless-stopped",
		"networks":          map[string]any{s.Network: map[string]any{"aliases": []string{db.ID}}},
		"labels":            databaseLabels(db, "database"),
		"environment":       env,
		"stop_grace_period": fmt.Sprintf("%ds", db.Settings.StopGracePeriod),
	}
	command, health, extra := dbCommand(db, s.Env)
	if len(command) > 0 {
		svc["command"] = command
	}
	if db.Settings.HealthCheckEnabled {
		svc["healthcheck"] = map[string]any{"test": health, "interval": "5s", "timeout": "5s", "retries": 20, "start_period": "10s"}
	}
	for k, v := range extra {
		svc[k] = v
	}
	if db.PortsMappings != "" {
		var ports []string
		for _, p := range strings.Split(db.PortsMappings, ",") {
			ports = append(ports, strings.TrimSpace(p))
		}
		svc["ports"] = ports
	}
	applyLimits(svc, db.Limits)
	mounts, named := volumesFor(s.Storages, s.Dir+"/files")
	var binds []any
	for _, m := range mounts {
		binds = append(binds, m)
	}
	if db.CustomConfig != "" && t.ConfigPath != "" {
		binds = append(binds, map[string]any{"type": "bind", "source": s.Dir + "/custom.conf", "target": t.ConfigPath, "read_only": true})
	}
	if sslEnabled(db) { // monté en écriture : les fichiers sont attribués au compte du moteur avant le démarrage
		binds = append(binds, map[string]any{"type": "bind", "source": s.Dir + "/ssl", "target": SSLTypes[db.Type].Dir})
	}
	if t.InitDir != "" {
		for _, sc := range db.InitScripts {
			binds = append(binds, map[string]any{"type": "bind", "source": s.Dir + "/initdb/" + sc.Filename,
				"target": t.InitDir + "/" + sc.Filename, "read_only": true})
		}
	}
	if len(binds) > 0 {
		svc["volumes"] = binds
	}
	if err := ApplyRunOptions(svc, db.CustomDockerRunOptions); err != nil {
		return "", err
	}
	doc := map[string]any{
		"services": map[string]any{"database": svc},
		"networks": map[string]any{s.Network: map[string]any{"external": true, "name": s.Network}},
	}
	if len(named) > 0 {
		doc["volumes"] = named
	}
	out, err := yaml.Marshal(doc)
	return string(out), err
}

// DatabaseProxy génère le compose et la configuration nginx (module stream) du proxy TCP qui publie
// la base sur son port public ; la résolution par le DNS de Docker suit les redémarrages de la base.
func DatabaseProxy(db *store.Database, network, dir string) (compose, nginx string, err error) {
	if db.PublicPort == nil {
		return "", "", errors.New("port public non défini")
	}
	port := strconv.Itoa(*db.PublicPort)
	nginx = fmt.Sprintf(`user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log;
events {
    worker_connections 1024;
}
stream {
    resolver 127.0.0.11 valid=10s;
    server {
        listen %s;
        set $upstream %s:%d;
        proxy_pass $upstream;
    }
}
`, port, db.ID, DBTypes[db.Type].Port)
	svc := map[string]any{
		"image": "nginx:stable-alpine", "container_name": db.ID + "-proxy", "restart": "unless-stopped",
		"networks": []string{network}, "labels": databaseLabels(db, "proxy"),
		"ports":   []string{port + ":" + port},
		"volumes": []any{map[string]any{"type": "bind", "source": dir + "/proxy/nginx.conf", "target": "/etc/nginx/nginx.conf", "read_only": true}},
	}
	out, err := yaml.Marshal(map[string]any{
		"services": map[string]any{"proxy": svc},
		"networks": map[string]any{network: map[string]any{"external": true, "name": network}},
	})
	return string(out), nginx, err
}

// --- Exécution sur le serveur ---

func (r *Runner) databasesDir() string {
	if r.DatabasesDir != "" {
		return r.DatabasesDir
	}
	return DatabasesDir
}

// dbRun : action en cours sur une base (sortie enregistrée dans la tâche serveur).
type dbRun struct {
	r         *Runner
	db        *store.Database
	t         ssh.Target
	out       io.Writer
	dir       string
	dockerEnv string // DOCKER_CONFIG des registres privés de l'équipe
}

func (d *dbRun) sh(ctx context.Context, label, script string) error {
	if label != "" {
		fmt.Fprintf(d.out, "$ %s\n", label)
	}
	code, err := d.r.SSH.RunInput(ctx, d.t, "sh -s", strings.NewReader("set -e\n"+d.dockerEnv+script), d.out, d.out)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("%s : échec (code %d)", label, code)
	}
	return nil
}

// registries active les registres privés de l'équipe pour les scripts suivants.
func (d *dbRun) registries(ctx context.Context) error {
	setup, env, err := d.r.dockerConfig(ctx, d.db.TeamID)
	if err != nil {
		return err
	}
	if err := d.sh(ctx, "", setup); err != nil {
		return err
	}
	d.dockerEnv = env
	return nil
}

// DatabaseTasks : tâches serveur des bases, à brancher dans servers.Service.Tasks.
func (r *Runner) DatabaseTasks() map[string]servers.TaskFunc {
	wrap := func(action func(context.Context, *dbRun) error) servers.TaskFunc {
		return func(ctx context.Context, task *store.ServerTask, sv *store.Server, t ssh.Target, out io.Writer) error {
			var p DatabaseParams
			if err := json.Unmarshal(task.Params, &p); err != nil {
				return err
			}
			db, err := r.Store.DatabaseByIDAny(ctx, p.DatabaseID)
			if err != nil {
				return fmt.Errorf("base %s : %w", p.DatabaseID, err)
			}
			if loc, err := r.Store.ResourceLocation(ctx, db.ID); err != nil || loc.ServerID != sv.ID {
				return errors.New("la base n'est pas sur ce serveur")
			}
			return action(ctx, &dbRun{r: r, db: db, t: t, out: out, dir: r.databasesDir() + "/" + db.ID})
		}
	}
	return map[string]servers.TaskFunc{
		TaskDatabaseStart: wrap(func(ctx context.Context, d *dbRun) error { return d.start(ctx) }),
		TaskDatabaseStop:  wrap(func(ctx context.Context, d *dbRun) error { return d.stop(ctx) }),
		TaskDatabaseRestart: wrap(func(ctx context.Context, d *dbRun) error {
			if err := d.stop(ctx); err != nil {
				return err
			}
			return d.start(ctx)
		}),
		TaskDatabaseProxy: wrap(func(ctx context.Context, d *dbRun) error { return d.syncProxy(ctx) }),
	}
}

// env déchiffre les identifiants et résout les variables de l'utilisateur (hors prévisualisation).
func (d *dbRun) env(ctx context.Context, loc *store.ResourceLocation) (map[string]string, error) {
	r, db := d.r, d.db
	pass, err := r.Box.Decrypt(db.PasswordEnc)
	if err != nil {
		return nil, err
	}
	root := ""
	if db.RootPasswordEnc != "" {
		if root, err = r.Box.Decrypt(db.RootPasswordEnc); err != nil {
			return nil, err
		}
	}
	raw, err := r.Store.SharedVariablesFor(ctx, db.TeamID, loc.ProjectID, loc.EnvironmentID, loc.ServerID)
	if err != nil {
		return nil, err
	}
	shared := make(map[string]string, len(raw))
	for k, v := range raw {
		if shared[k], err = r.Box.Decrypt(v.ValueEnc); err != nil {
			return nil, err
		}
	}
	list, err := r.Store.ListEnvVars(ctx, db.ID)
	if err != nil {
		return nil, err
	}
	user := map[string]string{}
	for _, v := range list {
		if v.IsPreview || !v.IsRuntime {
			continue
		}
		val, err := r.Box.Decrypt(v.ValueEnc)
		if err != nil {
			return nil, err
		}
		if !v.IsLiteral {
			var missing []string
			if val, missing = envvars.Interpolate(val, shared); len(missing) > 0 {
				fmt.Fprintf(d.out, "Attention : variables partagées introuvables (%s), remplacées par une valeur vide.\n", strings.Join(missing, ", "))
			}
		}
		user[v.Key] = val
	}
	return DatabaseEnv(db, pass, root, user), nil
}

func (d *dbRun) start(ctx context.Context) error {
	db, st := d.db, d.r.Store
	fmt.Fprintf(d.out, "Démarrage de %s (%s, %s).\n", db.Name, db.Type, db.Image)
	if err := d.registries(ctx); err != nil {
		return err
	}
	loc, err := st.ResourceLocation(ctx, db.ID)
	if err != nil {
		return err
	}
	env, err := d.env(ctx, loc)
	if err != nil {
		return err
	}
	storages, err := st.ListStorages(ctx, db.ID)
	if err != nil {
		return err
	}
	compose, err := DatabaseCompose(DatabaseSpec{DB: db, Env: env, Network: loc.Network, Storages: storages, Dir: d.dir})
	if err != nil {
		return err
	}
	// Fichiers : stockages, configuration personnalisée, scripts d'initialisation (régénérés à chaque démarrage).
	var files strings.Builder
	fmt.Fprintf(&files, "mkdir -p %s\nrm -rf %s\n", ssh.Quote(d.dir), ssh.Quote(d.dir+"/initdb"))
	for _, s := range storages {
		if s.Kind == "file" {
			files.WriteString(writeFile(d.dir+"/files/"+s.ID, s.Content, "644"))
		}
	}
	if db.CustomConfig != "" && DBTypes[db.Type].ConfigPath != "" {
		files.WriteString(writeFile(d.dir+"/custom.conf", db.CustomConfig, "644"))
	} else {
		fmt.Fprintf(&files, "rm -f %s\n", ssh.Quote(d.dir+"/custom.conf"))
	}
	if DBTypes[db.Type].InitDir != "" {
		for _, sc := range db.InitScripts {
			files.WriteString(writeFile(d.dir+"/initdb/"+sc.Filename, sc.Content, "644"))
		}
	}
	sv, err := st.ServerByIDAny(ctx, loc.ServerID)
	if err != nil {
		return err
	}
	certs, err := d.r.sslFiles(ctx, db, sv, d.dir)
	if err != nil {
		return err
	}
	files.WriteString(certs)
	file := d.dir + "/docker-compose.yml"
	files.WriteString(writeFile(file, compose, "600"))
	if err := d.sh(ctx, "", files.String()); err != nil {
		return err
	}
	base := "docker compose -p " + ssh.Quote(db.ID) + " -f " + ssh.Quote(file)
	if err := d.sh(ctx, "docker pull "+db.Image, base+" pull\n"); err != nil {
		return err
	}
	if sslEnabled(db) {
		fmt.Fprintln(d.out, "Certificats TLS signés par l'autorité du serveur.")
		if err := d.sh(ctx, "", sslOwnership(db, base, d.dir)); err != nil {
			return fmt.Errorf("droits des certificats : %w", err)
		}
	}
	if err := d.sh(ctx, "docker compose up -d", fmt.Sprintf("docker stop -t %d %s >/dev/null 2>&1 || true\ndocker rm -f %s >/dev/null 2>&1 || true\n%s up -d --remove-orphans\n",
		db.Settings.StopGracePeriod, ssh.Quote(db.ID), ssh.Quote(db.ID), base)); err != nil {
		return err
	}
	if err := d.waitHealthy(ctx); err != nil {
		_ = d.sh(context.WithoutCancel(ctx), "docker logs --tail 50 "+db.ID, "docker logs --tail 50 "+ssh.Quote(db.ID)+" 2>&1 || true\n")
		return err
	}
	if err := st.SetDatabaseStatus(ctx, db.ID, "running", true); err != nil {
		return err
	}
	if err := d.syncProxy(ctx); err != nil {
		return err
	}
	fmt.Fprintln(d.out, "Base démarrée.")
	return nil
}

// waitHealthy attend que la base soit saine (ou stable si la vérification de santé est désactivée).
func (d *dbRun) waitHealthy(ctx context.Context) error {
	fmt.Fprintln(d.out, "Attente du démarrage (jusqu'à 3 min)…")
	deadline := time.Now().Add(3 * time.Minute)
	stable := 0
	for time.Now().Before(deadline) {
		out, err := d.r.run(ctx, d.t, "docker inspect -f '{{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' "+ssh.Quote(d.db.ID)+"\n")
		if err != nil {
			return fmt.Errorf("conteneur introuvable : %w", err)
		}
		state, health, _ := strings.Cut(strings.TrimSpace(out), " ")
		switch {
		case state == "exited" || state == "dead" || health == "unhealthy":
			return fmt.Errorf("la base s'est arrêtée ou est en mauvaise santé (%s %s)", state, health)
		case state == "running" && health == "healthy":
			return nil
		case state == "running" && health == "":
			if stable++; stable >= 3 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return errors.New("la base n'est pas devenue saine à temps")
}

// stop arrête et supprime les conteneurs de la base et de son proxy (les volumes sont conservés).
func (d *dbRun) stop(ctx context.Context) error {
	fmt.Fprintf(d.out, "Arrêt de %s.\n", d.db.Name)
	if err := d.sh(ctx, "docker stop "+d.db.ID, fmt.Sprintf(`for c in $(docker ps -aq --filter label=nodra.resource=%s); do
  docker stop -t %d "$c" >/dev/null 2>&1 || true
  docker rm -f "$c" >/dev/null 2>&1 || true
done
`, d.db.ID, d.db.Settings.StopGracePeriod)); err != nil {
		return err
	}
	return d.r.Store.SetDatabaseStatus(ctx, d.db.ID, "stopped", false)
}

// syncProxy démarre le proxy public si la base est publique et démarrée, l'arrête sinon.
func (d *dbRun) syncProxy(ctx context.Context) error {
	db := d.db
	project := ssh.Quote(db.ID + "-proxy")
	cur, err := d.r.Store.DatabaseByIDAny(ctx, db.ID)
	if err != nil {
		return err
	}
	if !db.IsPublic || db.PublicPort == nil || cur.Status == "stopped" || cur.Status == "exited" {
		return d.sh(ctx, "", fmt.Sprintf("docker rm -f %s >/dev/null 2>&1 || true\nrm -rf %s\n", ssh.Quote(db.ID+"-proxy"), ssh.Quote(d.dir+"/proxy")))
	}
	loc, err := d.r.Store.ResourceLocation(ctx, db.ID)
	if err != nil {
		return err
	}
	compose, nginx, err := DatabaseProxy(db, loc.Network, d.dir)
	if err != nil {
		return err
	}
	file := d.dir + "/proxy/docker-compose.yml"
	script := writeFile(d.dir+"/proxy/nginx.conf", nginx, "644") + writeFile(file, compose, "600") +
		fmt.Sprintf("docker rm -f %s >/dev/null 2>&1 || true\ndocker compose -p %s -f %s up -d\n", ssh.Quote(db.ID+"-proxy"), project, ssh.Quote(file))
	if err := d.sh(ctx, fmt.Sprintf("proxy public sur le port %d", *db.PublicPort), script); err != nil {
		return err
	}
	fmt.Fprintf(d.out, "Base accessible publiquement sur le port %d.\n", *db.PublicPort)
	return nil
}

// TeardownDatabase supprime conteneurs et fichiers de la base sur le serveur (volumes si demandé).
// Un serveur injoignable n'empêche pas la suppression (les restes sont signalés dans les logs).
func (r *Runner) TeardownDatabase(ctx context.Context, db *store.Database, deleteVolumes bool) error {
	t, _, err := r.target(ctx, db.ID)
	if err != nil {
		r.Log.Warn("suppression : serveur inaccessible", "database", db.ID, "err", err)
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "for c in $(docker ps -aq --filter label=nodra.resource=%s); do docker rm -f \"$c\" >/dev/null 2>&1 || true; done\n", db.ID)
	if deleteVolumes {
		if storages, err := r.Store.ListStorages(ctx, db.ID); err == nil {
			for _, s := range storages {
				if s.Kind == "volume" {
					fmt.Fprintf(&b, "docker volume rm %s >/dev/null 2>&1 || true\n", ssh.Quote(s.Name))
				}
			}
		}
	}
	fmt.Fprintf(&b, "rm -rf %s\n", ssh.Quote(path.Join(r.databasesDir(), db.ID)))
	if _, err := r.run(ctx, t, b.String()); err != nil {
		r.Log.Warn("suppression : nettoyage incomplet", "database", db.ID, "err", err)
	}
	return nil
}

// DatabaseLogs renvoie les dernières lignes de logs de la base (et de son proxy public).
func (r *Runner) DatabaseLogs(ctx context.Context, db *store.Database, lines int) (string, error) {
	t, _, err := r.target(ctx, db.ID)
	if err != nil {
		return "", err
	}
	return r.run(ctx, t, fmt.Sprintf(`for c in $(docker ps -a --filter label=nodra.resource=%s --format '{{.Names}}'); do
  echo "==> $c"
  docker logs --tail %d --timestamps "$c" 2>&1
done
`, db.ID, lines))
}
