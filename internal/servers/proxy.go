package servers

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const (
	ProxyTraefik   = "traefik"
	ProxyCaddy     = "caddy"
	ProxyNone      = "none"
	ProxyContainer = "nodra-proxy"
	ProxyDir       = "/data/nodra/proxy"
	TraefikImage   = "traefik:v3.7"
	CaddyImage     = "lucaslorentz/caddy-docker-proxy:2.13-alpine"

	TaskProxyStart   = "proxy_start"
	TaskProxyStop    = "proxy_stop"
	TaskProxyRestart = "proxy_restart"
	TaskProxySync    = "proxy_sync"
)

func init() {
	handlers[TaskProxyStart] = proxyStart
	handlers[TaskProxyStop] = proxyStop
	handlers[TaskProxyRestart] = proxyRestart
	handlers[TaskProxySync] = proxySync
}

// ProxyConfigNameRe : fichiers de configuration dynamique autorisés (préfixe nodra- réservé).
var ProxyConfigNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}\.(ya?ml|caddy)$`)

// DefaultProxyCompose génère le docker-compose du proxy.
func DefaultProxyCompose(proxyType, acmeEmail string) string {
	switch proxyType {
	case ProxyTraefik:
		cmd := []string{
			"--ping=true",
			"--ping.entrypoint=http",
			"--api.dashboard=false",
			"--entrypoints.http.address=:80",
			"--entrypoints.https.address=:443",
			"--entrypoints.http.http.encodequerysemicolons=true",
			"--entryPoints.http.http2.maxConcurrentStreams=250",
			"--entrypoints.https.http.encodequerysemicolons=true",
			"--entryPoints.https.http2.maxConcurrentStreams=250",
			"--entrypoints.https.http3",
			"--providers.file.directory=/traefik/dynamic/",
			"--providers.file.watch=true",
			"--certificatesresolvers.letsencrypt.acme.httpchallenge=true",
			"--certificatesresolvers.letsencrypt.acme.httpchallenge.entrypoint=http",
			"--certificatesresolvers.letsencrypt.acme.storage=/traefik/acme.json",
			"--providers.docker=true",
			"--providers.docker.exposedbydefault=false",
			"--providers.docker.network=" + DefaultNetwork,
		}
		if acmeEmail != "" {
			cmd = append(cmd, "--certificatesresolvers.letsencrypt.acme.email="+acmeEmail)
		}
		var b strings.Builder
		b.WriteString(`name: nodra-proxy
networks:
  nodra:
    external: true
services:
  traefik:
    container_name: nodra-proxy
    image: ` + TraefikImage + `
    restart: unless-stopped
    extra_hosts:
      - host.docker.internal:host-gateway
    networks:
      - nodra
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    healthcheck:
      test: wget -qO- http://localhost:80/ping || exit 1
      interval: 4s
      timeout: 2s
      retries: 5
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ` + ProxyDir + `:/traefik
    command:
`)
		for _, c := range cmd {
			fmt.Fprintf(&b, "      - %q\n", c)
		}
		b.WriteString("    labels:\n      - traefik.enable=true\n      - nodra.managed=true\n      - nodra.proxy=true\n")
		return b.String()
	case ProxyCaddy:
		labels := "      - nodra.managed=true\n      - nodra.proxy=true\n"
		if acmeEmail != "" {
			labels += fmt.Sprintf("      - %q\n", "caddy.email="+acmeEmail)
		}
		return `name: nodra-proxy
networks:
  nodra:
    external: true
services:
  caddy:
    container_name: nodra-proxy
    image: ` + CaddyImage + `
    restart: unless-stopped
    extra_hosts:
      - host.docker.internal:host-gateway
    environment:
      - CADDY_DOCKER_POLLING_INTERVAL=5s
      - CADDY_DOCKER_CADDYFILE_PATH=/dynamic/Caddyfile
    networks:
      - nodra
    ports:
      - "80:80"
      - "443:443"
      - "443:443/udp"
    labels:
` + labels + `    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ` + ProxyDir + `/dynamic:/dynamic
      - ` + ProxyDir + `/config:/config
      - ` + ProxyDir + `/data:/data
      - ` + ProxyDir + `/certs:/certs:ro
`
	}
	return ""
}

// EffectiveCompose : configuration personnalisée si définie, sinon générée.
func EffectiveCompose(p store.ProxyInfo) string {
	if strings.TrimSpace(p.Compose) != "" {
		return p.Compose
	}
	return DefaultProxyCompose(p.Type, p.AcmeEmail)
}

// ValidateCompose vérifie qu'une configuration personnalisée est un YAML valide qui définit le conteneur nodra-proxy.
func ValidateCompose(s string) error {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(s), &doc); err != nil {
		return fmt.Errorf("YAML invalide : %w", err)
	}
	services, _ := doc["services"].(map[string]any)
	for _, svc := range services {
		if m, ok := svc.(map[string]any); ok && m["container_name"] == ProxyContainer {
			return nil
		}
	}
	return fmt.Errorf("un service doit avoir container_name: %s", ProxyContainer)
}

// ValidateDynamicConfig vérifie le format d'un fichier de configuration dynamique.
func ValidateDynamicConfig(proxyType, name, content string) error {
	isCaddy := strings.HasSuffix(name, ".caddy")
	if (proxyType == ProxyCaddy) != isCaddy {
		return fmt.Errorf("extension attendue pour %s : %s", proxyType, map[bool]string{true: ".caddy", false: ".yml / .yaml"}[proxyType == ProxyCaddy])
	}
	if strings.HasPrefix(name, "nodra-") {
		return fmt.Errorf("le préfixe nodra- est réservé")
	}
	if !isCaddy {
		var v any
		if err := yaml.Unmarshal([]byte(content), &v); err != nil {
			return fmt.Errorf("YAML invalide : %w", err)
		}
	}
	return nil
}

// proxyFiles renvoie les fichiers à écrire sur le serveur (chemin → contenu).
func (s *Service) proxyFiles(ctx context.Context, sv *store.Server) (map[string]string, error) {
	files := map[string]string{ProxyDir + "/docker-compose.yml": EffectiveCompose(sv.Proxy)}
	configs, err := s.Store.ListProxyConfigs(ctx, sv.ID)
	if err != nil {
		return nil, err
	}
	certs, err := s.Store.ListCertificates(ctx, sv.ID)
	if err != nil {
		return nil, err
	}
	for _, c := range certs {
		key, err := s.Box.Decrypt(c.PrivateKeyEnc)
		if err != nil {
			return nil, err
		}
		files[ProxyDir+"/certs/"+c.ID+".crt"] = c.Certificate
		files[ProxyDir+"/certs/"+c.ID+".key"] = key
	}
	switch sv.Proxy.Type {
	case ProxyTraefik:
		for _, c := range configs {
			files[ProxyDir+"/dynamic/"+c.Name] = c.Content
		}
		var b strings.Builder
		b.WriteString("# Généré par Nodra : certificats personnalisés.\ntls:\n  certificates:\n")
		if len(certs) == 0 {
			b.Reset()
			b.WriteString("# Généré par Nodra : aucun certificat personnalisé.\n{}\n")
		}
		for _, c := range certs {
			fmt.Fprintf(&b, "    - certFile: /traefik/certs/%s.crt\n      keyFile: /traefik/certs/%s.key\n", c.ID, c.ID)
		}
		files[ProxyDir+"/dynamic/nodra-certs.yml"] = b.String()
	case ProxyCaddy:
		for _, c := range configs {
			files[ProxyDir+"/dynamic/"+c.Name] = c.Content
		}
		files[ProxyDir+"/dynamic/Caddyfile"] = "# Généré par Nodra.\nimport /dynamic/*.caddy\n"
	}
	return files, nil
}

// writeFilesScript écrit les fichiers et supprime des dossiers gérés ceux qui ne sont plus attendus.
func writeFilesScript(files map[string]string, managedDirs []string) string {
	var b strings.Builder
	b.WriteString("set -e\nSUDO=\"\"; [ \"$(id -u)\" -ne 0 ] && SUDO=\"sudo -n\"\n")
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, d := range managedDirs {
		fmt.Fprintf(&b, "$SUDO mkdir -p %s\n", ssh.Quote(d))
		// Supprime les fichiers obsolètes du dossier (ceux qui ne figurent pas dans la liste).
		var keep []string
		for _, p := range paths {
			if strings.HasPrefix(p, d+"/") && !strings.Contains(strings.TrimPrefix(p, d+"/"), "/") {
				keep = append(keep, ssh.Quote(strings.TrimPrefix(p, d+"/")))
			}
		}
		fmt.Fprintf(&b, "for f in $($SUDO ls -A %s); do case \" %s \" in *\" '$f' \"*) ;; *) $SUDO rm -f %s/\"$f\";; esac; done\n",
			ssh.Quote(d), strings.Join(keep, " "), ssh.Quote(d))
	}
	for _, p := range paths {
		dir := p[:strings.LastIndex(p, "/")]
		fmt.Fprintf(&b, "$SUDO mkdir -p %s\nprintf %%s %s | base64 -d | $SUDO tee %s >/dev/null\n",
			ssh.Quote(dir), ssh.Quote(base64.StdEncoding.EncodeToString([]byte(files[p]))), ssh.Quote(p))
		if strings.HasSuffix(p, ".key") {
			fmt.Fprintf(&b, "$SUDO chmod 600 %s\n", ssh.Quote(p))
		}
	}
	return b.String()
}

func (r *taskRun) syncProxyFiles(ctx context.Context) error {
	files, err := r.svc.proxyFiles(ctx, r.server)
	if err != nil {
		return err
	}
	r.out.logf("Écriture de %d fichier(s) de configuration du proxy dans %s\n", len(files), ProxyDir)
	return r.script(ctx, "écriture de la configuration du proxy",
		writeFilesScript(files, []string{ProxyDir + "/dynamic", ProxyDir + "/certs"}))
}

// portsCheckScript échoue si les ports 80/443 sont occupés par autre chose que le proxy Nodra.
const portsCheckScript = `for p in 80 443; do
  owner=$(docker ps --filter "publish=$p" --format '{{.Names}}' | grep -v '^` + ProxyContainer + `$' || true)
  if [ -n "$owner" ]; then echo "Le port $p est utilisé par le conteneur $owner." >&2; exit 1; fi
  if command -v ss >/dev/null 2>&1 && [ -z "$(docker ps -q --filter name=^` + ProxyContainer + `$)" ] && ss -ltnH "sport = :$p" | grep -q .; then
    echo "Le port $p est déjà utilisé sur le serveur." >&2; exit 1
  fi
done
`

func (r *taskRun) proxyUp(ctx context.Context, recreate bool) error {
	if r.server.Proxy.Type == ProxyNone {
		return fmt.Errorf("aucun proxy configuré pour ce serveur")
	}
	if err := r.syncProxyFiles(ctx); err != nil {
		return err
	}
	if err := r.script(ctx, "vérification des ports 80 et 443", portsCheckScript); err != nil {
		return err
	}
	if err := r.exec(ctx, ensureNetwork(DefaultNetwork)); err != nil {
		return err
	}
	up := "docker compose -f " + ProxyDir + "/docker-compose.yml up -d --remove-orphans"
	if recreate {
		up += " --force-recreate"
	}
	if err := r.exec(ctx, up); err != nil {
		_ = r.svc.Store.SetProxyStatus(ctx, r.server.ID, "exited")
		return err
	}
	return r.svc.Store.SetProxyStatus(ctx, r.server.ID, "running")
}

func proxyStart(ctx context.Context, r *taskRun) error   { return r.proxyUp(ctx, false) }
func proxyRestart(ctx context.Context, r *taskRun) error { return r.proxyUp(ctx, true) }

func proxyStop(ctx context.Context, r *taskRun) error {
	if err := r.exec(ctx, "docker rm -f "+ProxyContainer+" >/dev/null 2>&1 || true"); err != nil {
		return err
	}
	return r.svc.Store.SetProxyStatus(ctx, r.server.ID, "stopped")
}

// proxySync réécrit la configuration (fichiers dynamiques, certificats) sans redémarrer le proxy :
// Traefik et caddy-docker-proxy surveillent leurs fichiers.
func proxySync(ctx context.Context, r *taskRun) error {
	if r.server.Proxy.Type == ProxyNone {
		return nil
	}
	return r.syncProxyFiles(ctx)
}

// InstanceContainer : conteneur de l'interface Nodra, joignable par le proxy sur le réseau nodra.
const InstanceContainer = "nodra:8000"

// InstanceProxyConfig : configuration dynamique qui sert l'interface de Nodra sur son domaine
// (serveur localhost). Renvoie le nom du fichier et son contenu ; nom vide sans proxy géré.
func InstanceProxyConfig(proxyType, fqdn string) (string, string) {
	u, err := url.Parse(fqdn)
	if err != nil || u.Hostname() == "" {
		return "", ""
	}
	host, https := u.Hostname(), u.Scheme == "https"
	switch proxyType {
	case ProxyTraefik:
		var b strings.Builder
		b.WriteString("http:\n")
		if https {
			b.WriteString("  middlewares:\n    nodra-instance-redirect:\n      redirectScheme:\n        scheme: https\n")
		}
		fmt.Fprintf(&b, "  routers:\n    nodra-instance-http:\n      rule: Host(`%s`)\n      entryPoints: [http]\n      service: nodra-instance\n", host)
		if https {
			b.WriteString("      middlewares: [nodra-instance-redirect]\n")
			fmt.Fprintf(&b, "    nodra-instance:\n      rule: Host(`%s`)\n      entryPoints: [https]\n      service: nodra-instance\n"+
				"      tls:\n        certResolver: letsencrypt\n", host)
		}
		fmt.Fprintf(&b, "  services:\n    nodra-instance:\n      loadBalancer:\n        servers:\n          - url: http://%s\n", InstanceContainer)
		return "nodra-instance.yaml", b.String()
	case ProxyCaddy:
		site := host
		if !https {
			site = "http://" + host
		}
		return "nodra-instance.caddy", fmt.Sprintf("%s {\n\treverse_proxy %s\n}\n", site, InstanceContainer)
	}
	return "", ""
}
