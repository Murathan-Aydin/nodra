// Package servers implémente les opérations distantes sur les serveurs (validation, installation de Docker,
// nettoyage, mises à jour, commandes, réseaux, métriques) exécutées en tâches de fond.
package servers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Types de tâches.
const (
	TaskValidate         = "validate"
	TaskInstallDocker    = "install_docker"
	TaskDockerCleanup    = "docker_cleanup"
	TaskPackagesCheck    = "packages_check"
	TaskPackagesUpdate   = "packages_update"
	TaskReboot           = "reboot"
	TaskCommand          = "command"
	TaskCloudflaredStart = "cloudflared_start"
	TaskCloudflaredStop  = "cloudflared_stop"
	TaskNetworkCreate    = "network_create"
	TaskNetworkDelete    = "network_delete"
)

const (
	DefaultNetwork       = "nodra"
	ManagedLabel         = "nodra.managed=true"
	cloudflaredContainer = "nodra-cloudflared"
	minDockerMajor       = 24
)

// SupportedOS : distributions sur lesquelles Nodra sait installer Docker.
var SupportedOS = []string{"ubuntu", "debian", "raspbian", "centos", "fedora", "rhel", "rocky", "almalinux", "ol",
	"amzn", "alpine", "arch", "manjaro", "opensuse-leap", "opensuse-tumbleweed", "sles", "pop", "linuxmint"}

var NetworkRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)

type Service struct {
	Store *store.Store
	Box   *crypto.Box
	SSH   *ssh.Pool
	Log   *slog.Logger
	// Tasks : types de tâches fournis par d'autres modules (actions sur les bases de données…).
	Tasks map[string]TaskFunc
}

// TaskFunc exécute une tâche fournie par un autre module ; out reçoit la sortie enregistrée avec la tâche.
type TaskFunc func(ctx context.Context, task *store.ServerTask, sv *store.Server, t ssh.Target, out io.Writer) error

// Target construit la cible SSH d'un serveur (clé privée déchiffrée).
func (s *Service) Target(ctx context.Context, sv *store.Server) (ssh.Target, error) {
	key, err := s.Store.PrivateKeyByID(ctx, sv.TeamID, sv.PrivateKeyID)
	if err != nil {
		return ssh.Target{}, fmt.Errorf("clé privée : %w", err)
	}
	pem, err := s.Box.Decrypt(key.PrivateKeyEnc)
	if err != nil {
		return ssh.Target{}, fmt.Errorf("déchiffrement de la clé : %w", err)
	}
	return ssh.Target{Host: sv.IP, Port: sv.Port, User: sv.User, Key: []byte(pem), HostKey: sv.HostKey,
		Timeout: time.Duration(sv.Settings.ConnectionTimeout) * time.Second}, nil
}

// --- Exécution des tâches ---

type taskRun struct {
	svc    *Service
	task   *store.ServerTask
	server *store.Server
	target ssh.Target
	out    *outputWriter
}

type handler func(ctx context.Context, r *taskRun) error

var handlers = map[string]handler{
	TaskValidate:         validate,
	TaskInstallDocker:    installDocker,
	TaskDockerCleanup:    dockerCleanup,
	TaskPackagesCheck:    packagesCheck,
	TaskPackagesUpdate:   packagesUpdate,
	TaskReboot:           reboot,
	TaskCommand:          command,
	TaskCloudflaredStart: cloudflaredStart,
	TaskCloudflaredStop:  cloudflaredStop,
	TaskNetworkCreate:    networkCreate,
	TaskNetworkDelete:    networkDelete,
}

func IsTaskKind(k string) bool { _, ok := handlers[k]; return ok }

// RunTask exécute une tâche en file et enregistre sa sortie et son statut.
func (s *Service) RunTask(ctx context.Context, id string) error {
	task, err := s.Store.ServerTaskByID(ctx, id)
	if err != nil {
		return err
	}
	if ok, err := s.Store.StartServerTask(ctx, id); err != nil || !ok {
		return err // annulée entre-temps
	}
	r := &taskRun{svc: s, task: task, out: newOutputWriter(s.Store, id)}
	err = func() error {
		h := handlers[task.Kind]
		if fn := s.Tasks[task.Kind]; h == nil && fn != nil {
			h = func(ctx context.Context, r *taskRun) error { return fn(ctx, r.task, r.server, r.target, r.out) }
		}
		if h == nil {
			return fmt.Errorf("type de tâche inconnu : %s", task.Kind)
		}
		if r.server, err = s.Store.ServerByIDAny(ctx, task.ServerID); err != nil {
			return err
		}
		if r.target, err = s.Target(ctx, r.server); err != nil {
			return err
		}
		if task.Kind != TaskValidate && r.target.HostKey == "" {
			return ssh.ErrNoHostKey
		}
		return h(ctx, r)
	}()
	r.out.Close()
	status, msg := "success", ""
	if err != nil {
		status, msg = "failed", err.Error()
		r.out.logf("Erreur : %s\n", msg)
		r.out.Close()
	}
	// Contexte détaché : le statut final doit être enregistré même si le job a expiré.
	return s.Store.FinishServerTask(context.WithoutCancel(ctx), id, status, msg)
}

func (r *taskRun) params(v any) error { return json.Unmarshal(r.task.Params, v) }

// exec affiche puis exécute une commande ; un code non nul est une erreur.
func (r *taskRun) exec(ctx context.Context, cmd string) error {
	return r.execAs(ctx, cmd, cmd)
}

// execAs exécute cmd en affichant label (pour masquer les secrets).
func (r *taskRun) execAs(ctx context.Context, cmd, label string) error {
	r.out.logf("$ %s\n", label)
	code, err := r.svc.SSH.Run(ctx, r.target, cmd, r.out, r.out)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("la commande a échoué (code %d)", code)
	}
	return nil
}

// script exécute un script shell complet (transmis en base64 pour éviter tout problème d'échappement).
func (r *taskRun) script(ctx context.Context, label, body string) error {
	cmd := "printf %s " + ssh.Quote(base64.StdEncoding.EncodeToString([]byte(body))) + " | base64 -d | sh"
	return r.execAs(ctx, cmd, label)
}

// sudo préfixe une commande par sudo si l'utilisateur SSH n'est pas root.
func (r *taskRun) sudo(cmd string) string {
	if r.server.User == "root" {
		return cmd
	}
	return "sudo -n sh -c " + ssh.Quote(cmd)
}

// outputWriter accumule la sortie et l'écrit en base par lots (au plus toutes les 500 ms).
type outputWriter struct {
	st     *store.Store
	id     string
	mu     sync.Mutex
	buf    strings.Builder
	last   time.Time
	closed bool
}

func newOutputWriter(st *store.Store, id string) *outputWriter {
	return &outputWriter{st: st, id: id, last: time.Now()}
}

func (w *outputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)
	if time.Since(w.last) > 500*time.Millisecond || w.buf.Len() > 16<<10 {
		w.flushLocked()
	}
	return len(p), nil
}

func (w *outputWriter) logf(format string, args ...any) { fmt.Fprintf(w, format, args...) }

func (w *outputWriter) flushLocked() {
	if w.buf.Len() == 0 {
		return
	}
	_ = w.st.AppendServerTaskOutput(context.Background(), w.id, w.buf.String())
	w.buf.Reset()
	w.last = time.Now()
}

func (w *outputWriter) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushLocked()
}

var _ io.Writer = (*outputWriter)(nil)

// --- Validation ---

func validate(ctx context.Context, r *taskRun) error {
	v := store.ServerValidation{}
	err := func() error {
		if r.target.HostKey == "" {
			r.out.logf("Première connexion : enregistrement de la clé d'hôte du serveur.\n")
			hk, err := ssh.FetchHostKey(ctx, r.target)
			if err != nil {
				return fmt.Errorf("connexion SSH impossible : %w", err)
			}
			if err := r.svc.Store.SetServerHostKey(ctx, r.server.ID, hk); err != nil {
				return err
			}
			r.target.HostKey = hk
			r.out.logf("Clé d'hôte : %s\n", hk)
		}
		if err := r.exec(ctx, "uptime"); err != nil {
			return fmt.Errorf("serveur injoignable : %w", err)
		}
		v.Reachable = true

		osRelease, err := r.svc.SSH.Output(ctx, r.target, "cat /etc/os-release")
		if err != nil {
			return fmt.Errorf("lecture de /etc/os-release : %w", err)
		}
		v.OSID, v.OSVersion = ParseOSRelease(osRelease)
		r.out.logf("Système : %s %s\n", v.OSID, v.OSVersion)
		if !isSupported(v.OSID) {
			return fmt.Errorf("système %q non pris en charge : installez Docker manuellement", v.OSID)
		}

		dv, err := r.svc.SSH.Output(ctx, r.target, "docker version --format '{{.Server.Version}}'")
		if err != nil {
			return errors.New("Docker n'est pas installé ou pas démarré : lancez l'installation de Docker")
		}
		v.DockerVersion = strings.TrimSpace(dv)
		r.out.logf("Docker : %s\n", v.DockerVersion)
		if major, _ := strconv.Atoi(strings.SplitN(v.DockerVersion, ".", 2)[0]); major < minDockerMajor {
			return fmt.Errorf("Docker %s trop ancien (%d minimum) : mettez-le à jour", v.DockerVersion, minDockerMajor)
		}
		cv, err := r.svc.SSH.Output(ctx, r.target, "docker compose version --short")
		if err != nil {
			return errors.New("le plugin Docker Compose est absent : lancez l'installation de Docker")
		}
		v.ComposeVersion = strings.TrimSpace(cv)
		r.out.logf("Docker Compose : %s\n", v.ComposeVersion)

		if err := r.exec(ctx, ensureNetwork(DefaultNetwork)); err != nil {
			return fmt.Errorf("création du réseau %s : %w", DefaultNetwork, err)
		}
		if _, err := r.svc.Store.UpsertDestination(ctx, r.server.ID, DefaultNetwork, DefaultNetwork, true); err != nil {
			return err
		}
		v.Usable = true
		r.out.logf("Serveur prêt.\n")
		// Démarrage automatique du proxy (non bloquant pour la validation).
		if r.server.Proxy.Type != ProxyNone && r.server.Proxy.Status != "running" {
			r.out.logf("Démarrage du proxy %s…\n", r.server.Proxy.Type)
			if err := r.proxyUp(ctx, false); err != nil {
				r.out.logf("Le proxy n'a pas pu démarrer : %s\n", err)
			}
		}
		return nil
	}()
	if err != nil {
		v.Error = err.Error()
		if errors.Is(err, ssh.ErrHostKeyMismatch) {
			v.Error = ssh.ErrHostKeyMismatch.Error() + " Réinitialisez la clé d'hôte si le serveur a été réinstallé."
		}
	}
	if serr := r.svc.Store.SetServerValidation(context.WithoutCancel(ctx), r.server.ID, v); serr != nil {
		return serr
	}
	return err
}

// ParseOSRelease extrait ID et VERSION_ID de /etc/os-release.
func ParseOSRelease(s string) (id, version string) {
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			id = strings.ToLower(v)
		case "VERSION_ID":
			version = v
		}
	}
	return id, version
}

func isSupported(id string) bool {
	for _, s := range SupportedOS {
		if s == id {
			return true
		}
	}
	return false
}

func ensureNetwork(name string) string {
	q := ssh.Quote(name)
	return "docker network inspect " + q + " >/dev/null 2>&1 || docker network create --attachable --label " + ManagedLabel + " " + q
}

// --- Installation de Docker ---

const installDockerScript = `set -e
SUDO=""
if [ "$(id -u)" -ne 0 ]; then SUDO="sudo -n"; fi
echo "Installation des prérequis (curl, wget, git, jq, openssl)…"
if command -v apt-get >/dev/null 2>&1; then
  $SUDO apt-get update -y
  DEBIAN_FRONTEND=noninteractive $SUDO apt-get install -y curl wget git jq openssl ca-certificates
elif command -v dnf >/dev/null 2>&1; then
  $SUDO dnf install -y curl wget git jq openssl
elif command -v yum >/dev/null 2>&1; then
  $SUDO yum install -y curl wget git jq openssl
elif command -v apk >/dev/null 2>&1; then
  $SUDO apk add --no-cache curl wget git jq openssl
elif command -v zypper >/dev/null 2>&1; then
  $SUDO zypper -n install curl wget git jq openssl
elif command -v pacman >/dev/null 2>&1; then
  $SUDO pacman -Sy --noconfirm curl wget git jq openssl
fi
if ! command -v docker >/dev/null 2>&1 || ! docker compose version >/dev/null 2>&1; then
  echo "Installation de Docker…"
  if command -v apk >/dev/null 2>&1; then
    $SUDO apk add --no-cache docker docker-cli-compose docker-cli-buildx
    $SUDO rc-update add docker default
  elif command -v pacman >/dev/null 2>&1; then
    $SUDO pacman -Sy --noconfirm docker docker-compose docker-buildx
  else
    curl -fsSL https://get.docker.com | $SUDO sh
  fi
fi
echo "Configuration de la rotation des logs Docker…"
$SUDO mkdir -p /etc/docker
CONF='{"log-driver":"json-file","log-opts":{"max-size":"10m","max-file":"3"}}'
if [ -s /etc/docker/daemon.json ]; then
  $SUDO cp /etc/docker/daemon.json "/etc/docker/daemon.json.bak-$(date +%Y%m%d%H%M%S)"
  echo "$CONF" | jq -s '.[0] * .[1]' - /etc/docker/daemon.json | $SUDO tee /etc/docker/daemon.json.new >/dev/null
  $SUDO mv /etc/docker/daemon.json.new /etc/docker/daemon.json
else
  echo "$CONF" | $SUDO tee /etc/docker/daemon.json >/dev/null
fi
if command -v systemctl >/dev/null 2>&1; then
  $SUDO systemctl enable docker
  $SUDO systemctl restart docker
else
  $SUDO service docker restart
fi
docker version
docker compose version
`

func installDocker(ctx context.Context, r *taskRun) error {
	if err := r.script(ctx, "installation de Docker", installDockerScript); err != nil {
		return err
	}
	r.out.logf("Docker installé : validation du serveur…\n")
	return validate(ctx, r)
}

// --- Nettoyage Docker ---

// CleanupCommands : commandes de nettoyage. Les conteneurs, images, volumes et réseaux marqués
// nodra.managed (ressources déployées, images conservées pour les retours arrière) sont épargnés.
func CleanupCommands(st store.ServerSettings) []string {
	cmds := []string{
		`docker container prune -f --filter "label!=` + ManagedLabel + `"`,
		`docker image prune -af --filter "label!=` + ManagedLabel + `"`,
		`docker builder prune -af`,
	}
	if st.DeleteUnusedVolumes {
		cmds = append(cmds, `docker volume prune -af --filter "label!=`+ManagedLabel+`"`)
	}
	if st.DeleteUnusedNetworks {
		cmds = append(cmds, `docker network prune -f --filter "label!=`+ManagedLabel+`"`)
	}
	return cmds
}

func dockerCleanup(ctx context.Context, r *taskRun) error {
	_ = r.exec(ctx, "df -h / | tail -1")
	for _, c := range CleanupCommands(r.server.Settings) {
		if err := r.exec(ctx, c); err != nil {
			return err
		}
	}
	_ = r.exec(ctx, "df -h / | tail -1")
	return r.svc.Store.SetServerCleanedUp(ctx, r.server.ID)
}

// --- Paquets ---

func packagesCheck(ctx context.Context, r *taskRun) error {
	return r.script(ctx, "recherche des mises à jour", `SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo -n"
if command -v apt-get >/dev/null 2>&1; then $SUDO apt-get update -qq >/dev/null && apt list --upgradable 2>/dev/null | tail -n +2
elif command -v dnf >/dev/null 2>&1; then $SUDO dnf check-update -q; [ $? -le 100 ]
elif command -v yum >/dev/null 2>&1; then $SUDO yum check-update -q; [ $? -le 100 ]
elif command -v apk >/dev/null 2>&1; then $SUDO apk update -q && apk -u list
elif command -v zypper >/dev/null 2>&1; then $SUDO zypper -q list-updates
elif command -v pacman >/dev/null 2>&1; then $SUDO pacman -Sy >/dev/null && pacman -Qu; true
else echo "gestionnaire de paquets inconnu" >&2; exit 1; fi
`)
}

func packagesUpdate(ctx context.Context, r *taskRun) error {
	return r.script(ctx, "mise à jour des paquets", `set -e
SUDO=""; [ "$(id -u)" -ne 0 ] && SUDO="sudo -n"
if command -v apt-get >/dev/null 2>&1; then $SUDO apt-get update -y && DEBIAN_FRONTEND=noninteractive $SUDO apt-get upgrade -y
elif command -v dnf >/dev/null 2>&1; then $SUDO dnf upgrade -y
elif command -v yum >/dev/null 2>&1; then $SUDO yum update -y
elif command -v apk >/dev/null 2>&1; then $SUDO apk upgrade --no-cache
elif command -v zypper >/dev/null 2>&1; then $SUDO zypper -n update
elif command -v pacman >/dev/null 2>&1; then $SUDO pacman -Syu --noconfirm
else echo "gestionnaire de paquets inconnu" >&2; exit 1; fi
`)
}

func reboot(ctx context.Context, r *taskRun) error {
	if err := r.exec(ctx, r.sudo("nohup sh -c 'sleep 2; reboot' >/dev/null 2>&1 &")); err != nil {
		return err
	}
	r.svc.SSH.Drop(r.target) // la connexion va tomber
	return r.svc.Store.SetServerReachable(ctx, r.server.ID, false)
}

// --- Commande libre ---

type CommandParams struct {
	Command string `json:"command"`
}

func command(ctx context.Context, r *taskRun) error {
	var p CommandParams
	if err := r.params(&p); err != nil {
		return err
	}
	return r.exec(ctx, p.Command)
}

// --- Cloudflare Tunnel ---

type CloudflaredParams struct {
	TokenEnc string `json:"token_enc"` // jeton chiffré
}

func cloudflaredStart(ctx context.Context, r *taskRun) error {
	var p CloudflaredParams
	if err := r.params(&p); err != nil {
		return err
	}
	token, err := r.svc.Box.Decrypt(p.TokenEnc)
	if err != nil {
		return err
	}
	_ = r.exec(ctx, "docker rm -f "+cloudflaredContainer+" >/dev/null 2>&1 || true")
	run := "docker run -d --name " + cloudflaredContainer + " --restart unless-stopped --network host --label " + ManagedLabel +
		" cloudflare/cloudflared:latest tunnel --no-autoupdate run --token "
	if err := r.execAs(ctx, run+ssh.Quote(token), run+"********"); err != nil {
		return err
	}
	return r.setTunnel(ctx, true)
}

func cloudflaredStop(ctx context.Context, r *taskRun) error {
	if err := r.exec(ctx, "docker rm -f "+cloudflaredContainer+" >/dev/null 2>&1 || true"); err != nil {
		return err
	}
	return r.setTunnel(ctx, false)
}

func (r *taskRun) setTunnel(ctx context.Context, on bool) error {
	sv, err := r.svc.Store.ServerByIDAny(ctx, r.server.ID)
	if err != nil {
		return err
	}
	sv.Settings.CloudflareTunnel = on
	return r.svc.Store.UpdateServer(ctx, sv)
}

// --- Réseaux (destinations) ---

type NetworkParams struct {
	Name          string `json:"name"`
	Network       string `json:"network"`
	DestinationID string `json:"destination_id,omitempty"`
}

func networkCreate(ctx context.Context, r *taskRun) error {
	var p NetworkParams
	if err := r.params(&p); err != nil {
		return err
	}
	if !NetworkRe.MatchString(p.Network) {
		return errors.New("nom de réseau invalide")
	}
	if err := r.exec(ctx, ensureNetwork(p.Network)); err != nil {
		return err
	}
	_, err := r.svc.Store.UpsertDestination(ctx, r.server.ID, p.Name, p.Network, false)
	return err
}

func networkDelete(ctx context.Context, r *taskRun) error {
	var p NetworkParams
	if err := r.params(&p); err != nil {
		return err
	}
	if err := r.svc.Store.DeleteDestination(ctx, r.server.ID, p.DestinationID); err != nil {
		return err
	}
	return r.exec(ctx, "docker network rm "+ssh.Quote(p.Network)+" 2>/dev/null || true")
}
