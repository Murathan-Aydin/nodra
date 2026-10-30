package ssh

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/ssh/sshtest"
)

func target(t *testing.T) (Target, *sshtest.Server) {
	priv, pub, err := GenerateKey("nodra-test")
	if err != nil {
		t.Fatal(err)
	}
	srv := sshtest.New(t, pub, sshtest.Local)
	return Target{Host: srv.Host, Port: srv.Port, User: "root", Key: []byte(priv)}, srv
}

func TestRunAndHostKey(t *testing.T) {
	ctx := context.Background()
	tg, srv := target(t)
	p := NewPool()
	defer p.Close()

	if _, err := p.Output(ctx, tg, "true"); err != ErrNoHostKey {
		t.Fatalf("sans clé d'hôte, refus attendu : %v", err)
	}
	hk, err := FetchHostKey(ctx, tg)
	if err != nil || hk != srv.AuthorizedHostKey() {
		t.Fatalf("clé d'hôte : %q %v", hk, err)
	}
	tg.HostKey = hk
	out, err := p.Output(ctx, tg, "echo bonjour")
	if err != nil || strings.TrimSpace(out) != "bonjour" {
		t.Fatalf("sortie %q, %v", out, err)
	}
	var stdout, stderr bytes.Buffer
	code, err := p.Run(ctx, tg, "echo erreur >&2; exit 3", &stdout, &stderr)
	if err != nil || code != 3 || strings.TrimSpace(stderr.String()) != "erreur" {
		t.Fatalf("code %d, stderr %q, %v", code, stderr.String(), err)
	}
	if _, err := p.Output(ctx, tg, "exit 1"); err == nil {
		t.Fatal("un code non nul doit être une erreur")
	}
	// Connexion réutilisée : plusieurs sessions en parallèle.
	done := make(chan error, 5)
	for range 5 {
		go func() { _, err := p.Output(ctx, tg, "echo x"); done <- err }()
	}
	for range 5 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	// Mauvaise clé d'hôte : refus.
	other, _ := target(t)
	bad := tg
	bad.HostKey = func() string { h, _ := FetchHostKey(ctx, other); return h }()
	if _, err := p.Output(ctx, bad, "true"); err != ErrHostKeyMismatch {
		t.Fatalf("clé d'hôte différente : refus attendu, %v", err)
	}
}

func TestCancel(t *testing.T) {
	tg, srv := target(t)
	srv.SetHandler(func(cmd string, stdout, stderr io.Writer) int {
		time.Sleep(3 * time.Second)
		return 0
	})
	tg.HostKey = srv.AuthorizedHostKey()
	p := NewPool()
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := p.Run(ctx, tg, "sleep 3", io.Discard, io.Discard); err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("l'annulation doit interrompre la commande (%v, %s)", err, time.Since(start))
	}
}

func TestKeys(t *testing.T) {
	priv, pub, err := GenerateKey("c")
	if err != nil {
		t.Fatal(err)
	}
	gotPub, fp, err := ParseKey(priv)
	if err != nil || !strings.HasPrefix(pub, gotPub) || !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("ParseKey : %q %q %v", gotPub, fp, err)
	}
	if _, _, err := ParseKey("n'importe quoi"); err == nil {
		t.Fatal("clé invalide acceptée")
	}
	if Quote("it's") != `'it'"'"'s'` {
		t.Fatal("échappement shell")
	}
}
