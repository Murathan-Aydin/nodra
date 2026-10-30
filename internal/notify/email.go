package notify

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// MailConfig : envoi des e-mails de l'instance (SMTP ou Resend).
type MailConfig struct {
	Provider     string `json:"provider"` // smtp | resend
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	Username     string `json:"username,omitempty"`
	Password     string `json:"password,omitempty"`
	Encryption   string `json:"encryption,omitempty"` // tls (465) | starttls (587) | none
	FromAddress  string `json:"from_address"`
	FromName     string `json:"from_name,omitempty"`
	ResendAPIKey string `json:"resend_api_key,omitempty"`
	ResendAPI    string `json:"-"` // remplaçable dans les tests
}

func (m MailConfig) Validate() error {
	if !strings.Contains(m.FromAddress, "@") || strings.ContainsAny(m.FromAddress+m.FromName, "\r\n<>") {
		return errors.New("from_address invalide")
	}
	switch m.Provider {
	case "smtp":
		if m.Host == "" || m.Port < 1 || m.Port > 65535 {
			return errors.New("host et port SMTP requis")
		}
		if m.Encryption != "tls" && m.Encryption != "starttls" && m.Encryption != "none" {
			return errors.New("encryption : tls, starttls ou none")
		}
	case "resend":
		if m.ResendAPIKey == "" {
			return errors.New("resend_api_key requis")
		}
	default:
		return errors.New("provider : smtp ou resend")
	}
	return nil
}

// Redacted masque les secrets.
func (m MailConfig) Redacted() MailConfig {
	if m.Password != "" {
		m.Password = "********"
	}
	if m.ResendAPIKey != "" {
		m.ResendAPIKey = "********"
	}
	return m
}

func (m MailConfig) from() string {
	if m.FromName == "" {
		return m.FromAddress
	}
	return mime.QEncoding.Encode("utf-8", m.FromName) + " <" + m.FromAddress + ">"
}

// Message construit un e-mail MIME texte (UTF-8, quoted-printable).
func (m MailConfig) Message(to []string, subject, text string, now time.Time) []byte {
	var b bytes.Buffer
	id := make([]byte, 12)
	_, _ = rand.Read(id)
	domain := m.FromAddress[strings.LastIndex(m.FromAddress, "@")+1:]
	headers := [][2]string{
		{"From", m.from()}, {"To", strings.Join(to, ", ")}, {"Subject", mime.QEncoding.Encode("utf-8", subject)},
		{"Date", now.Format(time.RFC1123Z)}, {"Message-ID", "<" + hex.EncodeToString(id) + "@" + domain + ">"},
		{"MIME-Version", "1.0"}, {"Content-Type", "text/plain; charset=utf-8"}, {"Content-Transfer-Encoding", "quoted-printable"},
	}
	for _, h := range headers {
		fmt.Fprintf(&b, "%s: %s\r\n", h[0], h[1])
	}
	b.WriteString("\r\n")
	qp := quotedprintable.NewWriter(&b)
	_, _ = qp.Write([]byte(strings.ReplaceAll(text, "\n", "\r\n")))
	_ = qp.Close()
	return b.Bytes()
}

// Send envoie un e-mail texte.
func (m MailConfig) Send(ctx context.Context, to []string, subject, text string) error {
	for _, r := range to {
		if strings.ContainsAny(r, "\r\n<>") {
			return fmt.Errorf("destinataire invalide : %s", r)
		}
	}
	if m.Provider == "resend" {
		return m.sendResend(ctx, to, subject, text)
	}
	return m.sendSMTP(ctx, to, subject, text)
}

func (m MailConfig) sendSMTP(ctx context.Context, to []string, subject, text string) error {
	addr := net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
	d := net.Dialer{Timeout: 15 * time.Second}
	var conn net.Conn
	var err error
	if m.Encryption == "tls" {
		conn, err = tls.DialWithDialer(&d, "tcp", addr, &tls.Config{ServerName: m.Host})
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("connexion SMTP : %w", err)
	}
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	c, err := smtp.NewClient(conn, m.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if m.Encryption == "starttls" {
		if err := c.StartTLS(&tls.Config{ServerName: m.Host}); err != nil {
			return fmt.Errorf("STARTTLS : %w", err)
		}
	}
	if m.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", m.Username, m.Password, m.Host)); err != nil {
			return fmt.Errorf("authentification SMTP : %w", err)
		}
	}
	if err := c.Mail(m.FromAddress); err != nil {
		return err
	}
	for _, r := range to {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("destinataire %s refusé : %w", r, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(m.Message(to, subject, text, time.Now())); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

func (m MailConfig) sendResend(ctx context.Context, to []string, subject, text string) error {
	api := m.ResendAPI
	if api == "" {
		api = "https://api.resend.com/emails"
	}
	from := m.FromAddress
	if m.FromName != "" {
		from = m.FromName + " <" + m.FromAddress + ">"
	}
	body, _ := json.Marshal(map[string]any{"from": from, "to": to, "subject": subject, "text": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, api, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Resend a répondu HTTP %d", resp.StatusCode)
	}
	return nil
}
