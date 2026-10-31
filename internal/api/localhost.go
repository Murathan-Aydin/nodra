package api

import (
	"context"
	"os"

	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// LocalhostConfig : accès SSH à l'hôte de l'instance, préparé par install.sh (clé ajoutée à authorized_keys).
type LocalhostConfig struct {
	KeyFile string // clé privée (vide : pas de serveur localhost)
	Host    string // host.docker.internal depuis le conteneur
	Port    int
	User    string
}

// BootstrapLocalhost crée une fois le serveur « localhost » dans l'équipe personnelle du premier
// administrateur, puis lance sa validation. Appelé au démarrage et après la première inscription.
func (s *Server) BootstrapLocalhost(ctx context.Context) error {
	c := s.Localhost
	if c.KeyFile == "" || s.Jobs == nil {
		return nil
	}
	teamID, userID, ok, err := s.Store.LocalhostTarget(ctx)
	if err != nil || !ok {
		return err
	}
	raw, err := os.ReadFile(c.KeyFile)
	if err != nil {
		return err
	}
	pub, fp, err := ssh.ParseKey(string(raw))
	if err != nil {
		return err
	}
	enc, err := s.Box.Encrypt(string(raw))
	if err != nil {
		return err
	}
	k, err := s.Store.CreatePrivateKey(ctx, &store.PrivateKey{TeamID: teamID, Name: "localhost",
		Description: "Clé du serveur hôte de Nodra (install.sh)", PrivateKeyEnc: enc, PublicKey: pub, Fingerprint: fp})
	if err != nil {
		return err
	}
	if c.Port == 0 {
		c.Port = 22
	}
	if c.User == "" {
		c.User = "root"
	}
	sv, err := s.Store.CreateServer(ctx, &store.Server{TeamID: teamID, Name: "localhost", Description: "Serveur hôte de Nodra",
		IP: c.Host, Port: c.Port, User: c.User, PrivateKeyID: k.ID, Settings: store.DefaultServerSettings(),
		Proxy: store.ProxyInfo{Type: servers.ProxyTraefik}})
	if err != nil {
		return err
	}
	if first, err := s.Store.MarkLocalhost(ctx, sv.ID); err != nil || !first {
		return err
	}
	_, err = s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: teamID, ServerID: sv.ID, Kind: servers.TaskValidate, CreatedBy: &userID})
	if err == nil {
		s.Log.Info("serveur localhost créé", "server", sv.ID, "host", c.Host)
	}
	return err
}
