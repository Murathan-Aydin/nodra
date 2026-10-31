package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Murathan-Aydin/nodra/internal/envvars"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/services"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// ServicesDir : dossier des services sur les serveurs.
const ServicesDir = "/data/nodra/services"

const (
	TaskServiceStart      = "service_start"
	TaskServiceStop       = "service_stop"
	TaskServiceRestart    = "service_restart"
	TaskServiceRestartApp = "service_restart_app"
)

// ServiceParams : paramètres d'une tâche serveur portant sur un service.
type ServiceParams struct {
	ServiceID string `json:"service_id"`
	App       string `json:"app,omitempty"`
}

func (r *Runner) servicesDir() string {
	if r.ServicesDir != "" {
		return r.ServicesDir
	}
	return ServicesDir
}

// ServiceSpec : données nécessaires à la génération du compose d'un service.
type ServiceSpec struct {
	Service   *store.Service
	Apps      []*store.ServiceApp
	Network   string
	ProxyType string
	DataDir   string
	Certs     map[string][2]string
}

// ServiceFile : fichier ou dossier à créer sur le serveur avant le démarrage (extensions de Coolify
// « content » et « isDirectory » des volumes, retirées du compose).
type ServiceFile struct {
	Path    string
	Content string
	Mode    string
	Dir     bool
}

var badInterpolationRe = regexp.MustCompile(`\$\{([^}]*)\}`)
var validInterpolationRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(:?[-?+].*)?$`)

// escapeInvalidInterpolation protège les ${...} qui ne sont pas des variables (ex. code JavaScript
// d'un healthcheck) : docker compose refuserait le fichier.
func escapeInvalidInterpolation(raw string) string {
	idx := badInterpolationRe.FindAllStringSubmatchIndex(raw, -1)
	var b strings.Builder
	last := 0
	for _, m := range idx {
		if (m[0] > 0 && raw[m[0]-1] == '$') || validInterpolationRe.MatchString(raw[m[2]:m[3]]) {
			continue
		}
		b.WriteString(raw[last:m[0]])
		b.WriteString("$")
		last = m[0]
	}
	b.WriteString(raw[last:])
	return b.String()
}

// ServiceCompose transforme le compose du service : variables déclarées sans valeur rendues explicites
// (KEY: ${KEY}, valeur prise dans le .env), nom de conteneur stable, labels de gestion et de routage,
// réseau de la destination, extensions de Coolify converties (fichiers à créer, exclusion du statut).
func ServiceCompose(s ServiceSpec) (string, []ServiceFile, error) {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(escapeInvalidInterpolation(s.Service.DockerComposeRaw)), &doc); err != nil {
		return "", nil, fmt.Errorf("docker-compose invalide : %w", err)
	}
	svcs, ok := doc["services"].(map[string]any)
	if !ok || len(svcs) == 0 {
		return "", nil, errors.New("docker-compose sans services")
	}
	var files []ServiceFile
	apps := map[string]*store.ServiceApp{}
	for _, a := range s.Apps {
		apps[a.Name] = a
	}
	for name, v := range svcs {
		svc, ok := v.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("service %s invalide", name)
		}
		exclude, _ := svc["exclude_from_hc"].(bool)
		delete(svc, "exclude_from_hc")
		f, err := extractFiles(svc, s.DataDir)
		if err != nil {
			return "", nil, fmt.Errorf("service %s : %w", name, err)
		}
		files = append(files, f...)
		svc["environment"] = explicitEnv(svc["environment"])
		svc["container_name"] = name + "-" + s.Service.ID
		labels := normalizeList(svc["labels"], "=")
		labels = append(labels, ManagedLabel, "nodra.resource="+s.Service.ID, "nodra.resource.type=service",
			"nodra.team="+s.Service.TeamID, "nodra.service="+name, "nodra.pr=0")
		if exclude && apps[name] == nil {
			labels = append(labels, "nodra.status=ignore")
		}
		if a := apps[name]; a != nil {
			if a.ExcludeFromStatus || exclude {
				labels = append(labels, "nodra.status=ignore")
			}
			if len(a.Domains) > 0 {
				port := a.Port
				if port == "" {
					port, _ = firstExposed(svc)
				}
				routing, err := Labels(RouteOptions{Name: s.Service.ID + "-" + name, Domains: a.Domains, Port: port, ProxyType: s.ProxyType,
					Network: s.Network, ForceHTTPS: true, Gzip: true, StripPrefix: true, Redirect: "both", CertResolver: "letsencrypt",
					CaddyCerts: s.Certs})
				if err != nil {
					return "", nil, err
				}
				labels = append(labels, escapeLabels(routing)...)
			}
		}
		svc["labels"] = labels
		attachService(svc, s.Network, s.DataDir)
	}
	attachNetwork(doc, s.Network)
	declareVolumes(doc)
	out, err := yaml.Marshal(doc)
	return string(out), files, err
}

// extractFiles retire les extensions content / isDirectory / is_directory / mode des volumes et renvoie
// les fichiers ou dossiers à créer. Seules les sources relatives (./…, sous dataDir) sont acceptées :
// une source absolue permettrait d'écrire n'importe où sur le serveur.
func extractFiles(svc map[string]any, dataDir string) ([]ServiceFile, error) {
	vols, _ := svc["volumes"].([]any)
	var files []ServiceFile
	for _, v := range vols {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		content, hasContent := m["content"]
		dirFlag := m["isDirectory"] == true || m["is_directory"] == true
		mode := fmt.Sprint(m["mode"])
		for _, k := range []string{"content", "isDirectory", "is_directory", "mode"} {
			delete(m, k)
		}
		if !hasContent && !dirFlag {
			continue
		}
		src, _ := m["source"].(string)
		if !strings.HasPrefix(src, "./") || strings.Contains(src, "..") {
			if !hasContent {
				continue // dossier d'un volume nommé ou d'un chemin absolu : Docker le crée lui-même
			}
			return nil, fmt.Errorf("volume %q : un fichier généré doit avoir une source relative (./…)", src)
		}
		f := ServiceFile{Path: dataDir + "/" + strings.TrimPrefix(src, "./"), Dir: dirFlag && !hasContent, Mode: "644"}
		if mode != "<nil>" && regexp.MustCompile(`^0?[0-7]{3}$`).MatchString(mode) {
			f.Mode = mode
		}
		if hasContent {
			f.Content = fmt.Sprint(content)
			if content == nil {
				f.Content = ""
			}
		}
		if m["type"] == nil {
			m["type"] = "bind"
		}
		files = append(files, f)
	}
	return files, nil
}

// InterpolateFile remplace ${VAR} et ${VAR:-défaut} par les valeurs du service ($$ donne $).
func InterpolateFile(content string, env map[string]string) string {
	re := regexp.MustCompile(`\$\$|\$\{([A-Za-z_][A-Za-z0-9_]*)(?::?-([^}]*))?\}`)
	return re.ReplaceAllStringFunc(content, func(m string) string {
		if m == "$$" {
			return "$"
		}
		sub := re.FindStringSubmatch(m)
		if v, ok := env[sub[1]]; ok && v != "" {
			return v
		}
		return sub[2]
	})
}

// explicitEnv convertit l'environnement en table ; une clé sans valeur devient KEY: ${KEY}.
func explicitEnv(env any) map[string]any {
	out := map[string]any{}
	switch e := env.(type) {
	case []any:
		for _, item := range e {
			s := fmt.Sprint(item)
			k, v, ok := strings.Cut(s, "=")
			if !ok {
				out[strings.TrimSpace(k)] = "${" + strings.TrimSpace(k) + "}"
				continue
			}
			out[k] = v
		}
	case map[string]any:
		for k, v := range e {
			if v == nil || v == "" {
				out[k] = "${" + k + "}"
			} else {
				out[k] = v
			}
		}
	}
	return out
}

// ServiceURLVars calcule SERVICE_URL_* et SERVICE_FQDN_* (avec et sans port) à partir du premier
// domaine de chaque sous-service ; les variables sans domaine valent "".
func ServiceURLVars(raw string, apps []*store.ServiceApp) map[string]string {
	byID := map[string]*url.URL{}
	for _, a := range apps {
		if a.URLVar == "" || len(a.Domains) == 0 {
			continue
		}
		if u, err := url.Parse(a.Domains[0]); err == nil {
			byID[a.URLVar] = u
		}
	}
	out := map[string]string{}
	an, err := services.Analyze(raw)
	if err != nil {
		return out
	}
	for name, m := range an.Magic {
		if m.Kind != "URL" && m.Kind != "FQDN" {
			continue
		}
		u := byID[m.ID]
		switch {
		case u == nil:
			out[name] = ""
		case m.Kind == "URL":
			out[name] = u.Scheme + "://" + u.Hostname() + strings.TrimSuffix(u.Path, "/")
		default:
			out[name] = u.Hostname()
		}
	}
	return out
}

// --- Tâches ---

type svcRun struct {
	r         *Runner
	svc       *store.Service
	t         ssh.Target
	out       io.Writer
	dir       string
	dockerEnv string // DOCKER_CONFIG des registres privés de l'équipe
}

func (d *svcRun) sh(ctx context.Context, label, script string) error {
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

func (d *svcRun) base() string {
	return fmt.Sprintf("docker compose -p %s -f %s --env-file %s", ssh.Quote(d.svc.ID), ssh.Quote(d.dir+"/docker-compose.yml"),
		ssh.Quote(d.dir+"/.env"))
}

// ServiceTasks : tâches serveur des services, à brancher dans servers.Service.Tasks.
func (r *Runner) ServiceTasks() map[string]servers.TaskFunc {
	wrap := func(action func(context.Context, *svcRun, ServiceParams) error) servers.TaskFunc {
		return func(ctx context.Context, task *store.ServerTask, sv *store.Server, t ssh.Target, out io.Writer) error {
			var p ServiceParams
			if err := json.Unmarshal(task.Params, &p); err != nil {
				return err
			}
			svc, err := r.Store.ServiceByIDAny(ctx, p.ServiceID)
			if err != nil {
				return fmt.Errorf("service %s : %w", p.ServiceID, err)
			}
			if loc, err := r.Store.ResourceLocation(ctx, svc.ID); err != nil || loc.ServerID != sv.ID {
				return errors.New("le service n'est pas sur ce serveur")
			}
			return action(ctx, &svcRun{r: r, svc: svc, t: t, out: out, dir: r.servicesDir() + "/" + svc.ID}, p)
		}
	}
	return map[string]servers.TaskFunc{
		TaskServiceStart: wrap(func(ctx context.Context, d *svcRun, _ ServiceParams) error { return d.start(ctx) }),
		TaskServiceStop:  wrap(func(ctx context.Context, d *svcRun, _ ServiceParams) error { return d.stop(ctx) }),
		TaskServiceRestart: wrap(func(ctx context.Context, d *svcRun, _ ServiceParams) error {
			if err := d.stop(ctx); err != nil {
				return err
			}
			return d.start(ctx)
		}),
		TaskServiceRestartApp: wrap(func(ctx context.Context, d *svcRun, p ServiceParams) error {
			return d.sh(ctx, "docker compose restart "+p.App, d.base()+" restart "+ssh.Quote(p.App)+"\n")
		}),
	}
}

// Tasks regroupe les tâches serveur fournies par le module de déploiement (bases et services).
func (r *Runner) Tasks() map[string]servers.TaskFunc {
	out := r.DatabaseTasks()
	for k, v := range r.ServiceTasks() {
		out[k] = v
	}
	out[TaskScheduled] = r.scheduledTask()
	return out
}

// resourceEnv déchiffre les variables d'une ressource et résout les variables partagées.
func (r *Runner) resourceEnv(ctx context.Context, resourceID, teamID string, loc *store.ResourceLocation, out io.Writer) (map[string]string, map[string]bool, error) {
	raw, err := r.Store.SharedVariablesFor(ctx, teamID, loc.ProjectID, loc.EnvironmentID, loc.ServerID)
	if err != nil {
		return nil, nil, err
	}
	shared := make(map[string]string, len(raw))
	for k, v := range raw {
		if shared[k], err = r.Box.Decrypt(v.ValueEnc); err != nil {
			return nil, nil, err
		}
	}
	list, err := r.Store.ListEnvVars(ctx, resourceID)
	if err != nil {
		return nil, nil, err
	}
	env, literal := map[string]string{}, map[string]bool{}
	for _, v := range list {
		if v.IsPreview || !v.IsRuntime {
			continue
		}
		val, err := r.Box.Decrypt(v.ValueEnc)
		if err != nil {
			return nil, nil, err
		}
		if v.IsLiteral {
			literal[v.Key] = true
		} else {
			var missing []string
			if val, missing = envvars.Interpolate(val, shared); len(missing) > 0 {
				fmt.Fprintf(out, "Attention : variables partagées introuvables (%s), remplacées par une valeur vide.\n", strings.Join(missing, ", "))
			}
		}
		env[v.Key] = val
	}
	return env, literal, nil
}

func (d *svcRun) start(ctx context.Context) error {
	r, svc := d.r, d.svc
	fmt.Fprintf(d.out, "Démarrage du service %s.\n", svc.Name)
	regSetup, regEnv, err := r.dockerConfig(ctx, svc.TeamID)
	if err != nil {
		return err
	}
	if err := d.sh(ctx, "", regSetup); err != nil {
		return err
	}
	d.dockerEnv = regEnv
	loc, err := r.Store.ResourceLocation(ctx, svc.ID)
	if err != nil {
		return err
	}
	sv, err := r.Store.ServerByIDAny(ctx, loc.ServerID)
	if err != nil {
		return err
	}
	apps, err := r.Store.ServiceApps(ctx, svc.ID)
	if err != nil {
		return err
	}
	env, literal, err := r.resourceEnv(ctx, svc.ID, svc.TeamID, loc, d.out)
	if err != nil {
		return err
	}
	for k, v := range ServiceURLVars(svc.DockerComposeRaw, apps) {
		env[k] = v
	}
	var certs map[string][2]string
	if sv.Proxy.Type == "caddy" {
		certs = map[string][2]string{}
		if list, err := r.Store.ListCertificates(ctx, sv.ID); err == nil {
			for _, c := range list {
				for _, dom := range c.Domains {
					certs[dom] = [2]string{"/certs/" + c.ID + ".crt", "/certs/" + c.ID + ".key"}
				}
			}
		}
	}
	compose, files, err := ServiceCompose(ServiceSpec{Service: svc, Apps: apps, Network: loc.Network, ProxyType: sv.Proxy.Type,
		DataDir: d.dir + "/data", Certs: certs})
	if err == nil && svc.LogDrainEnabled && r.Store.HasLogDrain(ctx, sv.ID) {
		compose, err = WithLogDrain(compose)
	}
	if err != nil {
		return err
	}
	var script strings.Builder
	fmt.Fprintf(&script, "mkdir -p %s\n", ssh.Quote(d.dir+"/data"))
	for _, f := range files {
		if f.Dir {
			fmt.Fprintf(&script, "mkdir -p %s\n", ssh.Quote(f.Path))
		} else {
			script.WriteString(writeFile(f.Path, InterpolateFile(f.Content, env), f.Mode))
		}
	}
	script.WriteString(writeFile(d.dir+"/docker-compose.yml", compose, "600") + writeFile(d.dir+"/.env", dotenv(env, literal), "600"))
	if err := d.sh(ctx, "", script.String()); err != nil {
		return err
	}
	if err := d.sh(ctx, "docker compose pull", d.base()+" pull --ignore-pull-failures\n"); err != nil {
		return err
	}
	if err := d.sh(ctx, "docker compose up -d", d.base()+" up -d --remove-orphans\n"); err != nil {
		return err
	}
	excluded := map[string]bool{}
	for _, a := range apps {
		excluded[a.Name] = a.ExcludeFromStatus
	}
	if err := d.wait(ctx, excluded); err != nil {
		_ = d.sh(context.WithoutCancel(ctx), "docker compose logs --tail 50", d.base()+" logs --tail 50 2>&1 || true\n")
		return err
	}
	if err := r.Store.SetServiceStatus(ctx, svc.ID, "running"); err != nil {
		return err
	}
	fmt.Fprintln(d.out, "Service démarré.")
	return nil
}

// wait attend que les sous-services soient démarrés (un service terminé avec le code 0 est accepté).
func (d *svcRun) wait(ctx context.Context, excluded map[string]bool) error {
	fmt.Fprintln(d.out, "Attente du démarrage des conteneurs (jusqu'à 5 min)…")
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		out, err := d.r.run(ctx, d.t, d.base()+" ps -a --format '{{.Service}} {{.State}} {{.Health}} {{.ExitCode}}'\n")
		if err != nil {
			return err
		}
		pending := false
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 || excluded[f[0]] {
				continue
			}
			state, health, code := f[1], "", "0"
			if len(f) == 4 {
				health, code = f[2], f[3]
			} else if len(f) == 3 {
				code = f[2]
			}
			switch {
			case state == "exited" && code != "0", state == "dead", health == "unhealthy":
				return fmt.Errorf("le conteneur %s s'est arrêté ou est en mauvaise santé", f[0])
			case state != "running" && state != "exited", health == "starting":
				pending = true
			}
		}
		if !pending {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	return errors.New("les conteneurs ne sont pas démarrés à temps")
}

func (d *svcRun) stop(ctx context.Context) error {
	fmt.Fprintf(d.out, "Arrêt du service %s.\n", d.svc.Name)
	if err := d.sh(ctx, "docker compose down", fmt.Sprintf(`if [ -f %s ]; then %s down --remove-orphans; fi
for c in $(docker ps -aq --filter label=nodra.resource=%s); do docker rm -f "$c" >/dev/null 2>&1 || true; done
`, ssh.Quote(d.dir+"/docker-compose.yml"), d.base(), d.svc.ID)); err != nil {
		return err
	}
	return d.r.Store.SetServiceStatus(ctx, d.svc.ID, "stopped")
}

// TeardownService supprime conteneurs et fichiers du service (volumes si demandé).
func (r *Runner) TeardownService(ctx context.Context, svc *store.Service, deleteVolumes bool) error {
	t, _, err := r.target(ctx, svc.ID)
	if err != nil {
		r.Log.Warn("suppression : serveur inaccessible", "service", svc.ID, "err", err)
		return nil
	}
	v := ""
	if deleteVolumes {
		v = " -v"
	}
	dir := r.servicesDir() + "/" + svc.ID
	script := fmt.Sprintf(`docker compose -p %s down --remove-orphans%s >/dev/null 2>&1 || true
for c in $(docker ps -aq --filter label=nodra.resource=%s); do docker rm -f "$c" >/dev/null 2>&1 || true; done
rm -rf %s
`, ssh.Quote(svc.ID), v, svc.ID, ssh.Quote(dir))
	if _, err := r.run(ctx, t, script); err != nil {
		r.Log.Warn("suppression : nettoyage incomplet", "service", svc.ID, "err", err)
	}
	return nil
}

// ServiceLogs renvoie les logs des conteneurs du service (d'un sous-service si app est fourni).
func (r *Runner) ServiceLogs(ctx context.Context, svc *store.Service, app string, lines int) (string, error) {
	t, _, err := r.target(ctx, svc.ID)
	if err != nil {
		return "", err
	}
	filter := "--filter label=nodra.resource=" + svc.ID
	if app != "" {
		filter += " --filter label=nodra.service=" + ssh.Quote(app)
	}
	return r.run(ctx, t, fmt.Sprintf(`for c in $(docker ps -a %s --format '{{.Names}}' | sort); do
  echo "==> $c"
  docker logs --tail %d --timestamps "$c" 2>&1
done
`, filter, lines))
}

// SortedKeys est utilisé par les tests pour comparer des tables.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
