package deploy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"path"

	"github.com/Murathan-Aydin/nodra/internal/ssh"
)

// dockerHubKey : clé des identifiants Docker Hub dans config.json.
const dockerHubKey = "https://index.docker.io/v1/"

// RegistryAuthKey : clé « auths » de config.json pour un hôte de registre.
func RegistryAuthKey(host string) string {
	switch host {
	case "docker.io", "index.docker.io", "registry-1.docker.io":
		return dockerHubKey
	}
	return host
}

// dockerConfig prépare les identifiants des registres privés de l'équipe sur le serveur : setup écrit
// <données>/docker-config/<équipe>/config.json (700/600, plugins de la CLI liés) et env l'active
// (DOCKER_CONFIG) pour les scripts suivants. Sans registre, setup retire une configuration obsolète.
func (r *Runner) dockerConfig(ctx context.Context, teamID string) (setup, env string, err error) {
	regs, err := r.Store.RegistriesFor(ctx, teamID)
	if err != nil {
		return "", "", err
	}
	dir := path.Dir(r.appsDir()) + "/docker-config/" + teamID
	q := ssh.Quote(dir)
	if len(regs) == 0 {
		return "rm -rf " + q + "\n", "", nil
	}
	auths := map[string]map[string]string{}
	for _, g := range regs {
		pw, err := r.Box.Decrypt(g.PasswordEnc)
		if err != nil {
			return "", "", err
		}
		auths[RegistryAuthKey(g.Host)] = map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(g.Username + ":" + pw))}
	}
	b, _ := json.Marshal(map[string]any{"auths": auths})
	setup = "mkdir -p " + q + " && chmod 700 " + q + "\n" +
		"(umask 077 && printf '%s' " + ssh.Quote(string(b)) + " > " + q + "/config.json)\n" +
		`if [ -d "$HOME/.docker/cli-plugins" ]; then ln -sfn "$HOME/.docker/cli-plugins" ` + q + "/cli-plugins; fi\n"
	return setup, "export DOCKER_CONFIG=" + q + "\n", nil
}
