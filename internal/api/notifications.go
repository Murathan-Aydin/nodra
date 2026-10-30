package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/auth"
	"github.com/Murathan-Aydin/nodra/internal/notify"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const verifyTTL = 24 * time.Hour

func (s *Server) notificationCatalog(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"events": notify.EventTypes, "channel_types": notify.ChannelTypes})
}

// channelView : canal avec configuration déchiffrée, secrets masqués.
func (s *Server) channelView(c *store.NotificationChannel) (map[string]any, error) {
	b, _ := json.Marshal(c)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	var cfg notify.Config
	raw, err := s.Box.Decrypt(c.ConfigEnc)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, err
	}
	m["config"] = cfg.Redacted()
	return m, nil
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListChannels(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, c := range list {
		v, err := s.channelView(c)
		if err != nil {
			s.internal(w, err)
			return
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

type channelInput struct {
	Type    string         `json:"type"`
	Name    *string        `json:"name"`
	Config  *notify.Config `json:"config"`
	Events  *[]string      `json:"events"`
	Enabled *bool          `json:"enabled"`
}

// applyChannel fusionne l'entrée (une valeur masquée ******** conserve le secret existant) et chiffre la configuration.
func (s *Server) applyChannel(in *channelInput, c *store.NotificationChannel, cfg *notify.Config) string {
	if in.Name != nil {
		c.Name = strings.TrimSpace(*in.Name)
	}
	if in.Config != nil {
		cfg.Merge(*in.Config)
	}
	if in.Events != nil {
		c.Events = []string{}
		for _, e := range *in.Events {
			if !notify.IsEventType(e) {
				return "événement inconnu : " + e
			}
			if !slices.Contains(c.Events, e) {
				c.Events = append(c.Events, e)
			}
		}
	}
	if in.Enabled != nil {
		c.Enabled = *in.Enabled
	}
	if c.Name == "" || len(c.Name) > 100 {
		return "nom requis (100 caractères max.)"
	}
	if err := cfg.Validate(c.Type); err != nil {
		return err.Error()
	}
	b, _ := json.Marshal(cfg)
	enc, err := s.Box.Encrypt(string(b))
	if err != nil {
		return "chiffrement impossible"
	}
	c.ConfigEnc = enc
	return ""
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "notification.manage", rbac.Scope{}) {
		return
	}
	var in channelInput
	if !decode(w, r, &in) {
		return
	}
	if !slices.Contains(notify.ChannelTypes, in.Type) {
		writeError(w, http.StatusUnprocessableEntity, "type : email, discord, slack, telegram, pushover ou webhook")
		return
	}
	c := &store.NotificationChannel{TeamID: teamFrom(r.Context()).Team.ID, Type: in.Type, Events: notify.DefaultEvents(), Enabled: true}
	var cfg notify.Config
	if msg := s.applyChannel(&in, c, &cfg); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	created, err := s.Store.CreateChannel(r.Context(), c)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "notification.channel.created", "notification_channel", created.ID, map[string]any{"type": c.Type, "name": c.Name, "events": c.Events})
	v, err := s.channelView(created)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, v)
}

func (s *Server) teamChannel(w http.ResponseWriter, r *http.Request, perm string) (*store.NotificationChannel, bool) {
	if !s.authorize(w, r, perm, rbac.Scope{}) {
		return nil, false
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	c, err := s.Store.ChannelByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return c, true
}

func (s *Server) updateChannel(w http.ResponseWriter, r *http.Request) {
	c, ok := s.teamChannel(w, r, "notification.manage")
	if !ok {
		return
	}
	var in channelInput
	if !decode(w, r, &in) {
		return
	}
	cfg, err := s.Notifier.ChannelConfig(c)
	if err != nil {
		s.internal(w, err)
		return
	}
	if msg := s.applyChannel(&in, c, &cfg); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if err := s.Store.UpdateChannel(r.Context(), c); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "notification.channel.updated", "notification_channel", c.ID, map[string]any{"name": c.Name, "events": c.Events, "enabled": c.Enabled})
	v, err := s.channelView(c)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	c, ok := s.teamChannel(w, r, "notification.manage")
	if !ok {
		return
	}
	if err := s.Store.DeleteChannel(r.Context(), c.TeamID, c.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "notification.channel.deleted", "notification_channel", c.ID, map[string]string{"name": c.Name})
	w.WriteHeader(http.StatusNoContent)
}

// testChannel envoie immédiatement un message de test et renvoie le résultat.
func (s *Server) testChannel(w http.ResponseWriter, r *http.Request) {
	c, ok := s.teamChannel(w, r, "notification.manage")
	if !ok {
		return
	}
	err := s.Notifier.Deliver(r.Context(), c, notify.Event{Type: notify.Test, TeamID: c.TeamID, Title: "Notification de test",
		Message: "Le canal « " + c.Name + " » de Nodra fonctionne.", Level: "info", At: time.Now()})
	if err != nil {
		writeError(w, http.StatusBadGateway, "envoi impossible : "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

func (s *Server) listDeliveries(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	list, err := s.Store.ListDeliveries(r.Context(), teamFrom(r.Context()).Team.ID, limit)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// --- E-mail de l'instance (administrateur) ---

func requireInstanceAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := userFrom(r.Context()); u == nil || !u.IsInstanceAdmin || tokenFrom(r.Context()) != nil {
			writeError(w, http.StatusForbidden, "réservé à l'administrateur de l'instance (session interactive)")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) getMailConfig(w http.ResponseWriter, r *http.Request) {
	m, err := s.Notifier.MailConfig(r.Context())
	if err != nil {
		s.internal(w, err)
		return
	}
	if m == nil {
		writeJSON(w, http.StatusOK, map[string]any{"configured": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"configured": true, "config": m.Redacted()})
}

func (s *Server) putMailConfig(w http.ResponseWriter, r *http.Request) {
	var in notify.MailConfig
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	if cur, err := s.Notifier.MailConfig(ctx); err == nil && cur != nil { // secrets masqués : conserver les anciens
		if in.Password == "********" {
			in.Password = cur.Password
		}
		if in.ResendAPIKey == "********" {
			in.ResendAPIKey = cur.ResendAPIKey
		}
	}
	if err := in.Validate(); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	b, _ := json.Marshal(in)
	enc, err := s.Box.Encrypt(string(b))
	if err != nil {
		s.internal(w, err)
		return
	}
	if err := s.Store.SetMailConfigEnc(ctx, enc); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "instance.mail.updated", "instance", "mail", map[string]string{"provider": in.Provider, "from": in.FromAddress})
	writeJSON(w, http.StatusOK, map[string]any{"configured": true, "config": in.Redacted()})
}

func (s *Server) deleteMailConfig(w http.ResponseWriter, r *http.Request) {
	if err := s.Store.SetMailConfigEnc(r.Context(), ""); err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "instance.mail.deleted", "instance", "mail", nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testMail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		To string `json:"to"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.To == "" {
		in.To = userFrom(r.Context()).Email
	}
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	if err := s.Notifier.SendMail(ctx, []string{in.To}, "Test d'envoi Nodra", "Cet e-mail de test confirme que l'envoi fonctionne."); err != nil {
		writeError(w, http.StatusBadGateway, "envoi impossible : "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

// --- Vérification de l'adresse e-mail ---

// sendVerification envoie un lien de vérification (si l'envoi d'e-mails est configuré).
func (s *Server) sendVerification(ctx context.Context, u *store.User) error {
	if s.Mailer == nil || u.EmailVerifiedAt != nil {
		return nil
	}
	if s.Notifier != nil {
		if m, err := s.Notifier.MailConfig(ctx); err != nil || m == nil {
			return err
		}
	}
	tok := auth.NewToken(auth.PrefixVerify)
	if err := s.Store.CreateEmailVerification(ctx, u.ID, auth.HashToken(tok), time.Now().Add(verifyTTL)); err != nil {
		return err
	}
	s.Mailer.EmailVerification(ctx, u.Email, tok)
	return nil
}

func (s *Server) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	uid, err := s.Store.ConsumeEmailVerification(r.Context(), auth.HashToken(in.Token))
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "lien de vérification invalide ou expiré")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	s.audit(r, nil, "auth.email.verified", "user", uid, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) resendVerification(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r.Context())
	if u.EmailVerifiedAt != nil {
		writeError(w, http.StatusConflict, "adresse déjà vérifiée")
		return
	}
	if err := s.sendVerification(r.Context(), u); err != nil {
		s.internal(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
