package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/ssh/sshtest"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func selfSigned(t *testing.T, domains []string, notAfter time.Time) (certPEM, keyPEM string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: domains[0]}, DNSNames: domains,
		NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}))
}

// scripts décode les scripts envoyés en base64 au serveur de test.
func scripts(srv *sshtest.Server) []string {
	var out []string
	for _, c := range srv.Received() {
		if rest, ok := strings.CutPrefix(c, "printf %s '"); ok {
			if b64, ok := strings.CutSuffix(rest, "' | base64 -d | sh"); ok {
				b, _ := base64.StdEncoding.DecodeString(b64)
				out = append(out, string(b))
			}
		}
	}
	return out
}

type fakeResolver map[string][]string

func (f fakeResolver) LookupHost(_ context.Context, host string) ([]string, error) {
	if ips, ok := f[host]; ok {
		return ips, nil
	}
	for k, ips := range f { // jokers : "*.apps.example.com"
		if strings.HasPrefix(k, "*.") && strings.HasSuffix(host, k[1:]) {
			return ips, nil
		}
	}
	return nil, errors.New("no such host")
}

func TestProxy(t *testing.T) {
	e := newTestEnv(t)
	alice, _ := register(t, e, "alice@example.com")
	bob, _ := register(t, e, "bob@example.com")
	var acme store.Team
	expect(t, "Acme", alice.do("POST", "/teams", map[string]string{"name": "Acme"}, &acme), 201)
	invite(t, e, alice, bob, acme.ID, "bob@example.com", rbac.RoleDeveloper)
	base := "/teams/" + acme.ID
	var key store.PrivateKey
	expect(t, "clé", alice.do("POST", base+"/private-keys", map[string]string{"name": "k"}, &key), 201)
	var dockerOK atomic.Bool
	dockerOK.Store(true)
	srv := sshtest.New(t, key.PublicKey, fakeUbuntu(&dockerOK))

	// La validation démarre automatiquement le proxy Traefik.
	var created struct {
		Server store.Server     `json:"server"`
		Task   store.ServerTask `json:"task"`
	}
	expect(t, "serveur", alice.do("POST", base+"/servers", map[string]any{"name": "s1", "ip": srv.Host, "port": srv.Port, "private_key_id": key.ID}, &created), 201)
	sid := created.Server.ID
	pbase := base + "/servers/" + sid + "/proxy"
	if task := waitTask(t, alice, acme.ID, sid, created.Task.ID); task.Status != "success" || !strings.Contains(task.Output, "docker compose -f /data/nodra/proxy/docker-compose.yml up -d") {
		t.Fatalf("démarrage du proxy attendu :\n%s", task.Output)
	}
	var proxy struct {
		Type, Status, Compose string
		IsCustom              bool `json:"is_custom"`
	}
	expect(t, "proxy", bob.do("GET", pbase, nil, &proxy), 200)
	if proxy.Type != "traefik" || proxy.Status != "running" || !strings.Contains(proxy.Compose, "traefik:v3.7") || proxy.IsCustom {
		t.Fatalf("proxy : %+v", proxy)
	}
	if s := scripts(srv); !slices.ContainsFunc(s, func(x string) bool { return strings.Contains(x, "/data/nodra/proxy/dynamic/nodra-certs.yml") }) {
		t.Fatal("la configuration doit être écrite sur le serveur")
	}

	expect(t, "développeur sans server.proxy.manage", bob.do("POST", pbase+"/restart", nil, nil), 403)
	expect(t, "changement de type en marche", alice.do("PATCH", pbase, map[string]any{"type": "caddy"}, nil), 409)

	// Configuration dynamique et certificats (Traefik)
	expect(t, "config invalide", alice.do("PUT", pbase+"/configs/redir.yml", map[string]string{"content": "http: ["}, nil), 422)
	expect(t, "extension caddy sur traefik", alice.do("PUT", pbase+"/configs/site.caddy", map[string]string{"content": "x"}, nil), 422)
	expect(t, "préfixe réservé", alice.do("PUT", pbase+"/configs/nodra-x.yml", map[string]string{"content": "{}"}, nil), 422)
	expect(t, "config", alice.do("PUT", pbase+"/configs/redir.yml", map[string]string{"content": "http:\n  routers: {}\n"}, nil), 204)
	var configs []store.ProxyConfig
	expect(t, "configs", alice.do("GET", pbase+"/configs", nil, &configs), 200)
	if len(configs) != 1 || configs[0].Name != "redir.yml" {
		t.Fatalf("configs : %+v", configs)
	}
	cert, certKey := selfSigned(t, []string{"app.example.com", "www.example.com"}, time.Now().Add(90*24*time.Hour))
	_, otherKey := selfSigned(t, []string{"x.example.com"}, time.Now().Add(time.Hour))
	expired, expiredKey := selfSigned(t, []string{"old.example.com"}, time.Now().Add(-time.Hour))
	expect(t, "clé ne correspondant pas", alice.do("POST", pbase+"/certificates", map[string]string{"name": "c", "certificate": cert, "private_key": otherKey}, nil), 422)
	expect(t, "certificat expiré", alice.do("POST", pbase+"/certificates", map[string]string{"name": "c", "certificate": expired, "private_key": expiredKey}, nil), 422)
	var c store.Certificate
	expect(t, "certificat", alice.do("POST", pbase+"/certificates", map[string]string{"name": "app", "certificate": cert, "private_key": certKey}, &c), 201)
	if !slices.Equal(c.Domains, []string{"app.example.com", "www.example.com"}) {
		t.Fatalf("domaines : %v", c.Domains)
	}
	// La synchronisation (tâche de fond) écrit le certificat et le référence dans la config Traefik.
	deadline := time.Now().Add(10 * time.Second)
	for {
		ok := slices.ContainsFunc(scripts(srv), func(x string) bool {
			return strings.Contains(x, "/data/nodra/proxy/certs/"+c.ID+".key") && strings.Contains(x, "/data/nodra/proxy/dynamic/redir.yml")
		})
		if ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("certificat non synchronisé sur le serveur")
		}
		time.Sleep(50 * time.Millisecond)
	}
	expect(t, "suppression certificat", alice.do("DELETE", pbase+"/certificates/"+c.ID, nil, nil), 204)

	// Arrêt, passage à Caddy, configuration personnalisée.
	var tk store.ServerTask
	expect(t, "arrêt", alice.do("POST", pbase+"/stop", nil, &tk), 202)
	waitTask(t, alice, acme.ID, sid, tk.ID)
	expect(t, "caddy", alice.do("PATCH", pbase, map[string]any{"type": "caddy", "acme_email": "ops@example.com"}, &proxy), 200)
	if proxy.Type != "caddy" || !strings.Contains(proxy.Compose, "caddy-docker-proxy") || !strings.Contains(proxy.Compose, "ops@example.com") {
		t.Fatalf("compose caddy : %+v", proxy)
	}
	expect(t, "e-mail invalide", alice.do("PATCH", pbase, map[string]any{"acme_email": "pas-un-email"}, nil), 422)
	expect(t, "compose invalide", alice.do("PATCH", pbase, map[string]any{"compose": "services:\n  x:\n    image: nginx\n"}, nil), 422)
	custom := strings.Replace(proxy.Compose, "CADDY_DOCKER_POLLING_INTERVAL=5s", "CADDY_DOCKER_POLLING_INTERVAL=10s", 1)
	expect(t, "compose personnalisé", alice.do("PATCH", pbase, map[string]any{"compose": custom}, &proxy), 200)
	if !proxy.IsCustom || !strings.Contains(proxy.Compose, "10s") {
		t.Fatal("configuration personnalisée")
	}
	expect(t, "retour au compose généré", alice.do("PATCH", pbase, map[string]any{"compose": ""}, &proxy), 200)
	if proxy.IsCustom {
		t.Fatal("retour à la configuration générée")
	}
	expect(t, "yml sur caddy", alice.do("PUT", pbase+"/configs/a.yml", map[string]string{"content": "a: b"}, nil), 422)
	expect(t, "site caddy", alice.do("PUT", pbase+"/configs/site.caddy", map[string]string{"content": "example.com {\n  respond ok\n}\n"}, nil), 204)
	expect(t, "démarrage caddy", alice.do("POST", pbase+"/start", nil, &tk), 202)
	if tk = waitTask(t, alice, acme.ID, sid, tk.ID); tk.Status != "success" {
		t.Fatalf("démarrage caddy : %s %s", tk.Status, tk.Error)
	}
	if !slices.ContainsFunc(scripts(srv), func(x string) bool { return strings.Contains(x, "/data/nodra/proxy/dynamic/Caddyfile") }) {
		t.Fatal("le Caddyfile d'import doit être écrit")
	}

	// Vérification DNS
	e.api.Resolver = fakeResolver{"app.example.com": {srv.Host}, "other.example.com": {"203.0.113.9"}, "*.apps.example.com": {srv.Host}}
	var dns struct {
		Domain string
		OK     bool
	}
	check := func(domain string) {
		t.Helper()
		expect(t, "dns "+domain, bob.do("POST", base+"/dns-check", map[string]string{"domain": domain, "server_id": sid}, &dns), 200)
	}
	check("https://App.example.com:8443/path")
	if !dns.OK || dns.Domain != "app.example.com" {
		t.Fatalf("dns ok attendu : %+v", dns)
	}
	check("other.example.com")
	if dns.OK {
		t.Fatal("other.example.com ne pointe pas vers le serveur")
	}
	check("absent.example.com")
	if dns.OK {
		t.Fatal("domaine absent")
	}
	check("*.apps.example.com")
	if !dns.OK {
		t.Fatal("joker")
	}
	expect(t, "domaine invalide", bob.do("POST", base+"/dns-check", map[string]string{"domain": "pas un domaine", "server_id": sid}, nil), 422)
}
