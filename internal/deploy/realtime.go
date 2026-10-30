package deploy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Container : conteneur d'une ressource sur son serveur.
type Container struct {
	Name    string `json:"name"`
	Service string `json:"service,omitempty"`
	State   string `json:"state"`
	Status  string `json:"status"`
	Image   string `json:"image"`
}

// Containers liste les conteneurs d'une ressource (pr : prévisualisation, 0 pour la production).
func (r *Runner) Containers(ctx context.Context, resourceID string, pr int) ([]Container, error) {
	t, _, err := r.target(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	out, err := r.run(ctx, t, fmt.Sprintf("docker ps -a --filter label=nodra.resource=%s --filter label=nodra.pr=%d --format %s\n",
		ssh.Quote(resourceID), pr, ssh.Quote(`{{.Names}}|{{.Label "nodra.service"}}|{{.State}}|{{.Status}}|{{.Image}}`)))
	if err != nil {
		return nil, err
	}
	list := []Container{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line == "" {
			continue
		}
		f := append(strings.Split(line, "|"), "", "", "", "")
		list = append(list, Container{Name: f[0], Service: f[1], State: f[2], Status: f[3], Image: f[4]})
	}
	slices.SortFunc(list, func(a, b Container) int { return strings.Compare(a.Name, b.Name) })
	return list, nil
}

// containerNames : noms des conteneurs de la ressource (hors PR), en cours d'exécution seulement si running.
func (r *Runner) containerNames(ctx context.Context, t ssh.Target, resourceID, service string, running bool) ([]string, error) {
	cmd := "docker ps -a " + ContainerFilter(ssh.Quote(resourceID), service)
	if running {
		cmd += " --filter status=running"
	}
	out, err := r.run(ctx, t, cmd+" --format '{{.Names}}'\n")
	if err != nil {
		return nil, err
	}
	names := strings.Fields(out)
	slices.Sort(names)
	return names, nil
}

// pick choisit le conteneur demandé parmi names, ou l'unique conteneur s'il n'est pas précisé.
func pick(names []string, container string) (string, error) {
	switch {
	case container != "":
		if !slices.Contains(names, container) {
			return "", fmt.Errorf("conteneur %q introuvable pour cette ressource", container)
		}
		return container, nil
	case len(names) == 0:
		return "", errors.New("aucun conteneur en cours d'exécution pour cette ressource")
	case len(names) > 1:
		return "", fmt.Errorf("plusieurs conteneurs (%s) : précisez le conteneur", strings.Join(names, ", "))
	}
	return names[0], nil
}

// FollowLogs suit les logs des conteneurs de la ressource (d'un sous-service ou d'un conteneur précis)
// jusqu'à l'annulation du contexte ; avec plusieurs conteneurs, chaque ligne est préfixée de son nom.
func (r *Runner) FollowLogs(ctx context.Context, resourceID, service, container string, tail int, w io.Writer) error {
	t, _, err := r.target(ctx, resourceID)
	if err != nil {
		return err
	}
	names, err := r.containerNames(ctx, t, resourceID, service, false)
	if err != nil {
		return err
	}
	if container != "" {
		if _, err := pick(names, container); err != nil {
			return err
		}
		names = []string{container}
	}
	logs := func(c string) string {
		return "docker logs -f --tail " + strconv.Itoa(tail) + " --timestamps " + ssh.Quote(c) + " 2>&1"
	}
	var script string
	switch len(names) {
	case 0:
		return errors.New("aucun conteneur pour cette ressource")
	case 1:
		script = "exec " + logs(names[0])
	default:
		var b strings.Builder
		for _, c := range names {
			fmt.Fprintf(&b, "(%s | sed %s) &\n", logs(c), ssh.Quote("s/^/["+c+"] /"))
		}
		script = b.String() + "wait"
	}
	return r.follow(ctx, t, script, w)
}

// FollowProxyLogs suit les logs du proxy du serveur.
func (r *Runner) FollowProxyLogs(ctx context.Context, sv *store.Server, tail int, w io.Writer) error {
	t, err := r.Servers.Target(ctx, sv)
	if err != nil {
		return err
	}
	return r.follow(ctx, t, "exec docker logs -f --tail "+strconv.Itoa(tail)+" --timestamps nodra-proxy 2>&1", w)
}

func (r *Runner) follow(ctx context.Context, t ssh.Target, script string, w io.Writer) error {
	code, err := r.SSH.Follow(ctx, t, "sh -c "+ssh.Quote(script), w)
	if err != nil || code == 0 {
		return err
	}
	return fmt.Errorf("le suivi des logs s'est arrêté (code %d)", code)
}

// ServerShell ouvre un shell interactif sur le serveur.
func (r *Runner) ServerShell(ctx context.Context, sv *store.Server, o ssh.ShellOptions) (*ssh.Shell, error) {
	t, err := r.Servers.Target(ctx, sv)
	if err != nil {
		return nil, err
	}
	return r.SSH.Shell(ctx, t, "", o)
}

// ContainerShell ouvre un shell (bash s'il existe, sinon sh) dans un conteneur en cours d'exécution
// de la ressource ; renvoie aussi le nom du conteneur.
func (r *Runner) ContainerShell(ctx context.Context, resourceID, service, container string, o ssh.ShellOptions) (*ssh.Shell, string, error) {
	t, _, err := r.target(ctx, resourceID)
	if err != nil {
		return nil, "", err
	}
	names, err := r.containerNames(ctx, t, resourceID, service, true)
	if err != nil {
		return nil, "", err
	}
	c, err := pick(names, container)
	if err != nil {
		return nil, "", err
	}
	script := "exec docker exec -it -e TERM=xterm-256color " + ssh.Quote(c) +
		" sh -c 'if command -v bash >/dev/null 2>&1; then exec bash; else exec sh; fi'"
	sh, err := r.SSH.Shell(ctx, t, "sh -c "+ssh.Quote(script), o)
	return sh, c, err
}
