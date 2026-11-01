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

func TestRunInput(t *testing.T) {
	tg, srv := target(t)
	tg.HostKey = srv.AuthorizedHostKey()
	p := NewPool()
	defer p.Close()
	var out bytes.Buffer
	code, err := p.RunInput(context.Background(), tg, "sh -s", strings.NewReader("echo depuis-stdin\nexit 4\n"), &out, io.Discard)
	if err != nil || code != 4 || strings.TrimSpace(out.String()) != "depuis-stdin" {
		t.Fatalf("code %d, sortie %q, %v", code, out.String(), err)
	}
}

func TestShell(t *testing.T) {
	ctx := context.Background()
	tg, srv := target(t)
	tg.HostKey = srv.AuthorizedHostKey()
	p := NewPool()
	defer p.Close()
	// Faux shell : renvoie chaque ligne reçue en majuscules, s'arrête sur « exit ».
	srv.SetInputHandler(func(cmd string, stdin io.Reader, stdout, _ io.Writer) int {
		buf := make([]byte, 256)
		for {
			n, err := stdin.Read(buf)
			line := strings.TrimSpace(string(buf[:n]))
			if line == "exit" || err != nil {
				return 7
			}
			_, _ = io.WriteString(stdout, strings.ToUpper(line)+"\n")
		}
	})
	sh, err := p.Shell(ctx, tg, "", ShellOptions{Rows: 30, Cols: 100})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(sh.Stdin, "bonjour\n")
	buf := make([]byte, 64)
	n, _ := sh.Stdout.Read(buf)
	if got := string(buf[:n]); got != "BONJOUR\n" {
		t.Fatalf("sortie %q", got)
	}
	if err := sh.Resize(40, 120); err != nil {
		t.Fatal(err)
	}
	if sh.Resize(0, 10) == nil {
		t.Fatal("taille invalide acceptée")
	}
	// window-change part sans accusé de réception : attendre que le serveur l'ait reçu avant de fermer.
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if p := srv.Ptys(); len(p) == 1 && len(p[0].Sizes) == 2 {
			break
		}
	}
	_, _ = io.WriteString(sh.Stdin, "exit\n")
	if code := sh.Wait(); code != 7 {
		t.Fatalf("code %d", code)
	}
	ptys := srv.Ptys()
	if len(ptys) != 1 || ptys[0].Term != "xterm-256color" || ptys[0].Command != "" ||
		len(ptys[0].Sizes) != 2 || ptys[0].Sizes[0] != [2]int{30, 100} || ptys[0].Sizes[1] != [2]int{40, 120} {
		t.Fatalf("pty : %+v", ptys)
	}

	// Follow : sortie copiée au fil de l'eau, arrêt par annulation du contexte.
	srv.SetInputHandler(func(cmd string, stdin io.Reader, stdout, _ io.Writer) int {
		_, _ = io.WriteString(stdout, "ligne 1\n")
		_, _ = io.Copy(io.Discard, stdin) // jusqu'à la coupure de la session
		return 0
	})
	cctx, cancel := context.WithCancel(ctx)
	pr, pw := io.Pipe()
	done := make(chan error, 1)
	go func() {
		_, err := p.Follow(cctx, tg, "docker logs -f x", pw)
		pw.Close()
		done <- err
	}()
	n, _ = pr.Read(buf)
	if string(buf[:n]) != "ligne 1\n" {
		t.Fatalf("follow : %q", buf[:n])
	}
	cancel()
	go func() { _, _ = io.Copy(io.Discard, pr) }()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("follow : %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follow non interrompu")
	}
}
