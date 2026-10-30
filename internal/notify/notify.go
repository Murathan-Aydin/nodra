package notify

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Publisher publie un événement (implémenté par les jobs : envoi asynchrone).
type Publisher interface {
	Publish(ctx context.Context, e Event)
}

// PublishFunc adapte une fonction en Publisher.
type PublishFunc func(ctx context.Context, e Event)

func (f PublishFunc) Publish(ctx context.Context, e Event) {
	if f != nil {
		f(ctx, e)
	}
}

// Dispatcher envoie les événements aux canaux abonnés et les e-mails de l'instance.
type Dispatcher struct {
	Store  *store.Store
	Box    *crypto.Box
	Log    *slog.Logger
	Sender Sender
	// ResendAPI : remplace l'API Resend (tests).
	ResendAPI string
	// BaseURL : URL publique de l'instance, préfixée aux liens relatifs des événements.
	BaseURL func(ctx context.Context) string
}

// MailConfig renvoie la configuration e-mail de l'instance (nil si non configurée).
func (d *Dispatcher) MailConfig(ctx context.Context) (*MailConfig, error) {
	enc, err := d.Store.MailConfigEnc(ctx)
	if err != nil || enc == "" {
		return nil, err
	}
	raw, err := d.Box.Decrypt(enc)
	if err != nil {
		return nil, err
	}
	var m MailConfig
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	m.ResendAPI = d.ResendAPI
	return &m, nil
}

// SendMail envoie un e-mail avec la configuration de l'instance.
func (d *Dispatcher) SendMail(ctx context.Context, to []string, subject, text string) error {
	m, err := d.MailConfig(ctx)
	if err != nil {
		return err
	}
	if m == nil {
		return errors.New("aucun envoi d'e-mail configuré sur l'instance")
	}
	return m.Send(ctx, to, subject, text)
}

func (d *Dispatcher) sender() *Sender {
	s := d.Sender
	s.Mail = d.SendMail
	return &s
}

// ChannelConfig déchiffre la configuration d'un canal.
func (d *Dispatcher) ChannelConfig(c *store.NotificationChannel) (Config, error) {
	var cfg Config
	raw, err := d.Box.Decrypt(c.ConfigEnc)
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal([]byte(raw), &cfg)
}

// Deliver envoie l'événement sur un canal et journalise le résultat.
func (d *Dispatcher) Deliver(ctx context.Context, c *store.NotificationChannel, e Event) error {
	cfg, err := d.ChannelConfig(c)
	if err == nil {
		sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err = d.sender().Send(sctx, c.Type, cfg, e)
		cancel()
	}
	status, msg := "sent", ""
	if err != nil {
		status, msg = "failed", err.Error()
		d.Log.Warn("notification", "channel", c.ID, "type", c.Type, "event", e.Type, "err", err)
	}
	if rerr := d.Store.RecordDelivery(context.WithoutCancel(ctx), c.TeamID, c.ID, e.Type, e.Title, status, msg); rerr != nil {
		d.Log.Error("journal des notifications", "err", rerr)
	}
	return err
}

// Dispatch envoie l'événement à tous les canaux abonnés (les échecs sont journalisés, jamais rejoués :
// un nouvel essai renverrait aussi aux canaux déjà servis).
func (d *Dispatcher) Dispatch(ctx context.Context, e Event) error {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if strings.HasPrefix(e.URL, "/") {
		base := ""
		if d.BaseURL != nil {
			base = strings.TrimSuffix(d.BaseURL(ctx), "/")
		}
		if base == "" {
			e.URL = ""
		} else {
			e.URL = base + e.URL
		}
	}
	channels, err := d.Store.SubscribedChannels(ctx, e.TeamID, e.Type)
	if err != nil {
		return err
	}
	for _, c := range channels {
		_ = d.Deliver(ctx, c, e)
	}
	return nil
}

// --- E-mails transactionnels ---

// Mailer : e-mails de réinitialisation, d'invitation et de vérification (envoyés en arrière-plan).
type Mailer struct {
	D       *Dispatcher
	BaseURL func(ctx context.Context) string
}

func (m *Mailer) send(ctx context.Context, to, subject, text string) {
	go func() {
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
		defer cancel()
		if err := m.D.SendMail(sctx, []string{to}, subject, text); err != nil {
			m.D.Log.Warn("e-mail non envoyé", "to", to, "subject", subject, "err", err)
		}
	}()
}

func (m *Mailer) PasswordReset(ctx context.Context, to, token string) {
	link := strings.TrimSuffix(m.BaseURL(ctx), "/") + "/reset-password?token=" + token
	m.send(ctx, to, "Réinitialisation de votre mot de passe Nodra",
		"Bonjour,\n\nPour choisir un nouveau mot de passe, ouvrez ce lien (valable 1 heure) :\n"+link+
			"\n\nSi vous n'êtes pas à l'origine de cette demande, ignorez cet e-mail.\n")
}

func (m *Mailer) TeamInvitation(ctx context.Context, to, teamName, token string) {
	link := strings.TrimSuffix(m.BaseURL(ctx), "/") + "/invitations/accept?token=" + token
	m.send(ctx, to, "Invitation à rejoindre l'équipe "+teamName+" sur Nodra",
		"Bonjour,\n\nVous êtes invité(e) à rejoindre l'équipe « "+teamName+" » sur Nodra.\nPour accepter (lien valable 7 jours) :\n"+link+"\n")
}

func (m *Mailer) EmailVerification(ctx context.Context, to, token string) {
	link := strings.TrimSuffix(m.BaseURL(ctx), "/") + "/verify-email?token=" + token
	m.send(ctx, to, "Confirmez votre adresse e-mail Nodra",
		"Bonjour,\n\nPour confirmer votre adresse e-mail, ouvrez ce lien (valable 24 heures) :\n"+link+"\n")
}
