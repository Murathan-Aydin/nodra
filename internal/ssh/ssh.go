// Package ssh exécute des commandes sur les serveurs gérés : pool de connexions réutilisées,
// vérification stricte de la clé d'hôte (mémorisée à la première connexion), annulation par contexte.
package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

type Target struct {
	Host    string
	Port    int
	User    string
	Key     []byte        // clé privée PEM
	HostKey string        // clé d'hôte attendue (format authorized_keys)
	Timeout time.Duration // délai de connexion (10 s par défaut)
}

func (t Target) addr() string { return net.JoinHostPort(t.Host, strconv.Itoa(t.Port)) }

func (t Target) poolKey() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|", t.addr(), t.User, t.HostKey)
	h.Write(t.Key)
	return hex.EncodeToString(h.Sum(nil))
}

var (
	ErrHostKeyMismatch = errors.New("la clé d'hôte du serveur ne correspond pas à celle enregistrée (serveur réinstallé ou attaque de l'homme du milieu ?)")
	ErrNoHostKey       = errors.New("clé d'hôte inconnue : valider le serveur d'abord")
)

type conn struct {
	client   *gossh.Client
	lastUsed time.Time
}

// Pool réutilise une connexion SSH par cible (multiplexage des sessions).
type Pool struct {
	mu    sync.Mutex
	conns map[string]*conn
	idle  time.Duration
	stop  chan struct{}
}

func NewPool() *Pool {
	p := &Pool{conns: map[string]*conn{}, idle: 5 * time.Minute, stop: make(chan struct{})}
	go p.janitor()
	return p
}

func (p *Pool) janitor() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.mu.Lock()
			for k, c := range p.conns {
				if time.Since(c.lastUsed) > p.idle {
					c.client.Close()
					delete(p.conns, k)
				}
			}
			p.mu.Unlock()
		}
	}
}

func (p *Pool) Close() {
	close(p.stop)
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, c := range p.conns {
		c.client.Close()
		delete(p.conns, k)
	}
}

// Drop ferme la connexion vers la cible (ex. avant un redémarrage du serveur).
func (p *Pool) Drop(t Target) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.conns[t.poolKey()]; ok {
		c.client.Close()
		delete(p.conns, t.poolKey())
	}
}

func dial(ctx context.Context, t Target, cb gossh.HostKeyCallback) (*gossh.Client, error) {
	signer, err := gossh.ParsePrivateKey(t.Key)
	if err != nil {
		return nil, fmt.Errorf("clé privée invalide : %w", err)
	}
	timeout := t.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	cfg := &gossh.ClientConfig{User: t.User, Auth: []gossh.AuthMethod{gossh.PublicKeys(signer)}, HostKeyCallback: cb, Timeout: timeout}
	d := net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	nc, err := d.DialContext(ctx, "tcp", t.addr())
	if err != nil {
		return nil, err
	}
	_ = nc.SetDeadline(time.Now().Add(timeout))
	c, chans, reqs, err := gossh.NewClientConn(nc, t.addr(), cfg)
	if err != nil {
		nc.Close()
		return nil, err
	}
	_ = nc.SetDeadline(time.Time{})
	return gossh.NewClient(c, chans, reqs), nil
}

func (p *Pool) client(ctx context.Context, t Target) (*gossh.Client, error) {
	if t.HostKey == "" {
		return nil, ErrNoHostKey
	}
	k := t.poolKey()
	p.mu.Lock()
	if c, ok := p.conns[k]; ok {
		c.lastUsed = time.Now()
		p.mu.Unlock()
		return c.client, nil
	}
	p.mu.Unlock()
	expected, _, _, _, err := gossh.ParseAuthorizedKey([]byte(t.HostKey))
	if err != nil {
		return nil, fmt.Errorf("clé d'hôte enregistrée invalide : %w", err)
	}
	cl, err := dial(ctx, t, func(_ string, _ net.Addr, key gossh.PublicKey) error {
		if !bytes.Equal(key.Marshal(), expected.Marshal()) {
			return ErrHostKeyMismatch
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrHostKeyMismatch) || strings.Contains(err.Error(), ErrHostKeyMismatch.Error()) {
			return nil, ErrHostKeyMismatch
		}
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.conns[k]; ok { // une autre goroutine a gagné la course
		cl.Close()
		c.lastUsed = time.Now()
		return c.client, nil
	}
	p.conns[k] = &conn{client: cl, lastUsed: time.Now()}
	return cl, nil
}

// FetchHostKey se connecte (en vérifiant l'authentification) et renvoie la clé d'hôte présentée.
// À n'utiliser que pour enregistrer la clé d'un nouveau serveur (confiance à la première connexion).
func FetchHostKey(ctx context.Context, t Target) (string, error) {
	var seen gossh.PublicKey
	cl, err := dial(ctx, t, func(_ string, _ net.Addr, key gossh.PublicKey) error {
		seen = key
		return nil
	})
	if err != nil {
		return "", err
	}
	cl.Close()
	return strings.TrimSpace(string(gossh.MarshalAuthorizedKey(seen))), nil
}

// Run exécute cmd et écrit stdout/stderr dans les writers fournis. Renvoie le code de sortie.
// L'annulation du contexte tue la commande distante.
func (p *Pool) Run(ctx context.Context, t Target, cmd string, stdout, stderr io.Writer) (int, error) {
	return p.RunInput(ctx, t, cmd, nil, stdout, stderr)
}

// RunInput est Run avec une entrée standard : un script passé par stdin (ex. "sh -s") n'apparaît pas
// dans la liste des processus du serveur, contrairement aux arguments de la commande.
func (p *Pool) RunInput(ctx context.Context, t Target, cmd string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	var sess *gossh.Session
	for attempt := 0; ; attempt++ {
		cl, err := p.client(ctx, t)
		if err != nil {
			return -1, err
		}
		if sess, err = cl.NewSession(); err == nil {
			break
		}
		p.Drop(t) // connexion morte : on retente une fois avec une nouvelle connexion
		if attempt == 1 {
			return -1, err
		}
	}
	defer sess.Close()
	sess.Stdout, sess.Stderr, sess.Stdin = stdout, stderr, stdin
	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()
	select {
	case err := <-done:
		var exit *gossh.ExitError
		if errors.As(err, &exit) {
			return exit.ExitStatus(), nil
		}
		if err != nil {
			p.Drop(t)
			return -1, err
		}
		return 0, nil
	case <-ctx.Done():
		_ = sess.Signal(gossh.SIGKILL)
		sess.Close()
		return -1, ctx.Err()
	}
}

// Output exécute cmd et renvoie stdout ; un code de sortie non nul devient une erreur contenant stderr.
func (p *Pool) Output(ctx context.Context, t Target, cmd string) (string, error) {
	var out, errb bytes.Buffer
	code, err := p.Run(ctx, t, cmd, &out, &errb)
	if err != nil {
		return "", err
	}
	if code != 0 {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.String(), fmt.Errorf("code de sortie %d : %s", code, msg)
	}
	return out.String(), nil
}

// --- Clés ---

// GenerateKey crée une paire ed25519 : clé privée PEM (OpenSSH) et clé publique au format authorized_keys.
func GenerateKey(comment string) (private, public string, err error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	block, err := gossh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return "", "", err
	}
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return "", "", err
	}
	return string(pem.EncodeToMemory(block)), authorizedKey(sshPub, comment), nil
}

// ParseKey valide une clé privée importée et renvoie sa clé publique et son empreinte SHA256.
func ParseKey(private string) (public, fingerprint string, err error) {
	signer, err := gossh.ParsePrivateKey([]byte(private))
	var pp *gossh.PassphraseMissingError
	if errors.As(err, &pp) {
		return "", "", errors.New("les clés protégées par une phrase de passe ne sont pas prises en charge")
	}
	if err != nil {
		return "", "", errors.New("clé privée invalide (format PEM/OpenSSH attendu)")
	}
	return authorizedKey(signer.PublicKey(), ""), gossh.FingerprintSHA256(signer.PublicKey()), nil
}

func authorizedKey(k gossh.PublicKey, comment string) string {
	s := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(k)))
	if comment != "" {
		s += " " + comment
	}
	return s
}

// Quote protège une chaîne pour l'utiliser comme argument d'une commande shell POSIX.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
