package notify

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// smtpServer : serveur SMTP minimal (sans TLS ni authentification) qui conserve les messages reçus.
type smtpServer struct {
	addr string
	mu   sync.Mutex
	msgs []string
	rcpt []string
}

func newSMTP(t *testing.T) *smtpServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s := &smtpServer{addr: ln.Addr().String()}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c)
		}
	}()
	return s
}

func (s *smtpServer) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := func(line string) { io.WriteString(c, line+"\r\n") }
	w("220 test")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			w("250-test")
			w("250 8BITMIME")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			s.mu.Lock()
			s.rcpt = append(s.rcpt, strings.Trim(strings.TrimSpace(line)[8:], "<>"))
			s.mu.Unlock()
			w("250 ok")
		case cmd == "DATA":
			w("354 go")
			var b strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil || l == ".\r\n" {
					break
				}
				b.WriteString(l)
			}
			s.mu.Lock()
			s.msgs = append(s.msgs, b.String())
			s.mu.Unlock()
			w("250 queued")
		case cmd == "QUIT":
			w("221 bye")
			return
		default:
			w("250 ok")
		}
	}
}

func TestSMTPMessage(t *testing.T) {
	srv := newSMTP(t)
	host, port, _ := net.SplitHostPort(srv.addr)
	p := 0
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	m := MailConfig{Provider: "smtp", Host: host, Port: p, Encryption: "none", FromAddress: "nodra@example.com", FromName: "Équipe Nodra"}
	if err := m.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := m.Send(context.Background(), []string{"alice@example.com"}, "Déploiement réussi ✅", "Bonjour Élodie,\nligne 2"); err != nil {
		t.Fatal(err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.msgs) != 1 || srv.rcpt[0] != "alice@example.com" {
		t.Fatalf("messages : %v %v", srv.msgs, srv.rcpt)
	}
	msg := srv.msgs[0]
	header, body, _ := strings.Cut(msg, "\r\n\r\n")
	dec := new(mime.WordDecoder)
	for _, l := range strings.Split(header, "\r\n") {
		if strings.HasPrefix(l, "Subject: ") {
			subj, _ := dec.DecodeHeader(strings.TrimPrefix(l, "Subject: "))
			if subj != "Déploiement réussi ✅" {
				t.Fatalf("sujet : %q", subj)
			}
		}
	}
	text, _ := io.ReadAll(quotedprintable.NewReader(strings.NewReader(body)))
	if !strings.Contains(string(text), "Bonjour Élodie,\r\nligne 2") || !strings.Contains(header, "charset=utf-8") {
		t.Fatalf("corps : %q", text)
	}
	if (MailConfig{Provider: "smtp", Host: "h", Port: 25, Encryption: "ssl", FromAddress: "a@b"}).Validate() == nil {
		t.Fatal("chiffrement inconnu")
	}
	if m.Send(context.Background(), []string{"x@y\r\nBcc: z@w"}, "s", "t") == nil {
		t.Fatal("injection d'en-tête refusée")
	}
}

func TestChannels(t *testing.T) {
	type hit struct {
		path, body, ctype string
	}
	var mu sync.Mutex
	var hits []hit
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		hits = append(hits, hit{r.URL.Path, string(b), r.Header.Get("Content-Type")})
		mu.Unlock()
		if strings.Contains(r.URL.Path, "fail") {
			w.WriteHeader(500)
			io.WriteString(w, "secret interne")
		}
	}))
	defer srv.Close()
	s := &Sender{TelegramAPI: srv.URL, PushoverAPI: srv.URL + "/pushover"}
	e := Event{Type: DeploymentFailure, Title: "Échec du déploiement", Message: "Site : build en échec", Level: "error",
		URL: "https://nodra.example.com/x", Fields: map[string]string{"Commit": "abc"}, At: time.Now()}
	ctx := context.Background()
	for typ, cfg := range map[string]Config{
		Discord:  {WebhookURL: srv.URL + "/discord", Ping: true},
		Slack:    {WebhookURL: srv.URL + "/slack"},
		Telegram: {BotToken: "123:abc", ChatID: "-100", Threads: map[string]string{DeploymentFailure: "7"}},
		Pushover: {UserKey: "u", APIToken: "tok"},
		Webhook:  {WebhookURL: srv.URL + "/hook"},
	} {
		if err := cfg.Validate(typ); err != nil {
			t.Fatalf("%s : %v", typ, err)
		}
		if err := s.Send(ctx, typ, cfg, e); err != nil {
			t.Fatalf("%s : %v", typ, err)
		}
	}
	got := map[string]hit{}
	for _, h := range hits {
		got[h.path] = h
	}
	var discord map[string]any
	_ = json.Unmarshal([]byte(got["/discord"].body), &discord)
	if discord["content"] != "@here" || discord["embeds"].([]any)[0].(map[string]any)["color"].(float64) != 0xe74c3c {
		t.Fatalf("discord : %s", got["/discord"].body)
	}
	if !strings.Contains(got["/slack"].body, "*❌ Échec du déploiement*") {
		t.Fatalf("slack : %s", got["/slack"].body)
	}
	var tg map[string]any
	_ = json.Unmarshal([]byte(got["/bot123:abc/sendMessage"].body), &tg)
	if tg["chat_id"] != "-100" || tg["message_thread_id"] != "7" || !strings.Contains(tg["text"].(string), "Commit : abc") {
		t.Fatalf("telegram : %v", tg)
	}
	form, _ := url.ParseQuery(got["/pushover"].body)
	if form.Get("token") != "tok" || form.Get("priority") != "1" || got["/pushover"].ctype != "application/x-www-form-urlencoded" {
		t.Fatalf("pushover : %v", form)
	}
	var hook Event
	_ = json.Unmarshal([]byte(got["/hook"].body), &hook)
	if hook.Type != DeploymentFailure || hook.Fields["Commit"] != "abc" {
		t.Fatalf("webhook : %s", got["/hook"].body)
	}
	err := s.Send(ctx, Webhook, Config{WebhookURL: srv.URL + "/fail"}, e)
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("erreur sans corps de réponse : %v", err)
	}
	for typ, cfg := range map[string]Config{Discord: {WebhookURL: "ftp://x"}, Telegram: {BotToken: "x"}, Email: {Recipients: []string{"pas-une-adresse"}}} {
		if cfg.Validate(typ) == nil {
			t.Errorf("%s : configuration invalide acceptée", typ)
		}
	}
	red := Config{WebhookURL: "https://secret", BotToken: "t"}.Redacted()
	if red.WebhookURL != "********" || red.BotToken != "********" {
		t.Fatal("secrets masqués")
	}
	c := Config{WebhookURL: "https://ancien"}
	c.Merge(Config{WebhookURL: "********"})
	if c.WebhookURL != "https://ancien" {
		t.Fatal("une valeur masquée conserve l'ancienne")
	}
}

func TestResend(t *testing.T) {
	var body map[string]any
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
	}))
	defer srv.Close()
	m := MailConfig{Provider: "resend", ResendAPIKey: "re_x", FromAddress: "nodra@example.com", FromName: "Nodra", ResendAPI: srv.URL}
	if err := m.Send(context.Background(), []string{"a@b.c"}, "Sujet", "Texte"); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer re_x" || body["from"] != "Nodra <nodra@example.com>" || body["subject"] != "Sujet" {
		t.Fatalf("resend : %s %v", auth, body)
	}
}
