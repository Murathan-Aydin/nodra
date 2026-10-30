package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Types de canaux.
const (
	Email    = "email"
	Discord  = "discord"
	Slack    = "slack"
	Telegram = "telegram"
	Pushover = "pushover"
	Webhook  = "webhook"
)

var ChannelTypes = []string{Email, Discord, Slack, Telegram, Pushover, Webhook}

// Config : configuration d'un canal (stockée chiffrée).
type Config struct {
	// E-mail : destinataires (l'expéditeur et le serveur SMTP sont ceux de l'instance).
	Recipients []string `json:"recipients,omitempty"`
	// Discord, Slack, webhook
	WebhookURL string `json:"webhook_url,omitempty"`
	// Discord : mentionner @here sur les erreurs
	Ping bool `json:"ping,omitempty"`
	// Telegram
	BotToken string            `json:"bot_token,omitempty"`
	ChatID   string            `json:"chat_id,omitempty"`
	Threads  map[string]string `json:"threads,omitempty"` // événement → identifiant de sujet (forum)
	// Pushover
	UserKey  string `json:"user_key,omitempty"`
	APIToken string `json:"api_token,omitempty"`
}

// Secrets : champs à masquer dans les réponses de l'API.
func (c Config) Redacted() Config {
	mask := func(s string) string {
		if s == "" {
			return ""
		}
		return "********"
	}
	c.WebhookURL, c.BotToken, c.APIToken, c.UserKey = mask(c.WebhookURL), mask(c.BotToken), mask(c.APIToken), mask(c.UserKey)
	return c
}

// Merge applique les champs fournis ; une valeur masquée (********) conserve l'ancienne.
func (c *Config) Merge(in Config) {
	keep := func(dst *string, v string) {
		if v != "" && v != "********" {
			*dst = v
		}
	}
	keep(&c.WebhookURL, in.WebhookURL)
	keep(&c.BotToken, in.BotToken)
	keep(&c.APIToken, in.APIToken)
	keep(&c.UserKey, in.UserKey)
	keep(&c.ChatID, in.ChatID)
	if in.Recipients != nil {
		c.Recipients = in.Recipients
	}
	if in.Threads != nil {
		c.Threads = in.Threads
	}
	c.Ping = in.Ping
}

// Validate vérifie la configuration selon le type de canal.
func (c Config) Validate(typ string) error {
	httpsURL := func(s string) bool {
		u, err := url.Parse(s)
		return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
	}
	switch typ {
	case Email:
		if len(c.Recipients) == 0 {
			return errors.New("au moins un destinataire")
		}
		for _, r := range c.Recipients {
			if !strings.Contains(r, "@") || strings.ContainsAny(r, " \r\n<>") {
				return fmt.Errorf("adresse invalide : %s", r)
			}
		}
	case Discord, Slack, Webhook:
		if !httpsURL(c.WebhookURL) {
			return errors.New("webhook_url : URL http(s) requise")
		}
	case Telegram:
		if c.BotToken == "" || c.ChatID == "" {
			return errors.New("bot_token et chat_id requis")
		}
	case Pushover:
		if c.UserKey == "" || c.APIToken == "" {
			return errors.New("user_key et api_token requis")
		}
	default:
		return fmt.Errorf("type de canal inconnu : %s", typ)
	}
	return nil
}

// Sender envoie un événement sur un canal.
type Sender struct {
	HTTP        *http.Client
	TelegramAPI string // https://api.telegram.org
	PushoverAPI string // https://api.pushover.net/1/messages.json
	Mail        func(ctx context.Context, to []string, subject, text string) error
}

func (s *Sender) client() *http.Client {
	if s.HTTP != nil {
		return s.HTTP
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func (s *Sender) post(ctx context.Context, rawURL, contentType string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "Nodra")
	resp, err := s.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("réponse HTTP %d", resp.StatusCode) // corps non renvoyé (URL configurable : anti-SSRF)
	}
	return nil
}

func (s *Sender) postJSON(ctx context.Context, rawURL string, v any) error {
	b, _ := json.Marshal(v)
	return s.post(ctx, rawURL, "application/json", b)
}

// Send envoie l'événement sur le canal.
func (s *Sender) Send(ctx context.Context, typ string, c Config, e Event) error {
	title := strings.TrimSpace(e.emoji() + " " + e.Title)
	switch typ {
	case Email:
		if s.Mail == nil {
			return errors.New("e-mail non configuré sur l'instance")
		}
		return s.Mail(ctx, c.Recipients, "[Nodra] "+e.Title, e.Text())
	case Discord:
		embed := map[string]any{"title": e.Title, "description": e.Message, "color": e.color(), "timestamp": e.At.Format(time.RFC3339)}
		if e.URL != "" {
			embed["url"] = e.URL
		}
		var fields []map[string]any
		for _, k := range sortedKeys(e.Fields) {
			fields = append(fields, map[string]any{"name": k, "value": e.Fields[k], "inline": true})
		}
		if len(fields) > 0 {
			embed["fields"] = fields
		}
		payload := map[string]any{"username": "Nodra", "embeds": []any{embed}}
		if c.Ping && e.Level == "error" {
			payload["content"] = "@here"
		}
		return s.postJSON(ctx, c.WebhookURL, payload)
	case Slack:
		return s.postJSON(ctx, c.WebhookURL, map[string]any{"text": "*" + title + "*\n" + e.Text()})
	case Telegram:
		api := s.TelegramAPI
		if api == "" {
			api = "https://api.telegram.org"
		}
		msg := map[string]any{"chat_id": c.ChatID, "text": title + "\n\n" + e.Text(), "disable_web_page_preview": true}
		if th := c.Threads[e.Type]; th != "" {
			msg["message_thread_id"] = th
		}
		return s.postJSON(ctx, api+"/bot"+c.BotToken+"/sendMessage", msg)
	case Pushover:
		api := s.PushoverAPI
		if api == "" {
			api = "https://api.pushover.net/1/messages.json"
		}
		form := url.Values{"token": {c.APIToken}, "user": {c.UserKey}, "title": {title}, "message": {e.Text()}}
		if e.URL != "" {
			form.Set("url", e.URL)
		}
		if e.Level == "error" {
			form.Set("priority", "1")
		}
		return s.post(ctx, api, "application/x-www-form-urlencoded", []byte(form.Encode()))
	case Webhook:
		return s.postJSON(ctx, c.WebhookURL, e)
	}
	return fmt.Errorf("type de canal inconnu : %s", typ)
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
