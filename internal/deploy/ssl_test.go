package deploy

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestIssueCert(t *testing.T) {
	caCert, caKey, err := GenerateCA("Nodra CA test")
	if err != nil {
		t.Fatal(err)
	}
	cert, key, err := IssueCert(caCert, caKey, "db-123", []string{"localhost", "127.0.0.1", "203.0.113.5"})
	if err != nil || !strings.Contains(key, "PRIVATE KEY") {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM([]byte(caCert))
	b, _ := pem.Decode([]byte(cert))
	c, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"db-123", "localhost", "127.0.0.1", "203.0.113.5"} {
		if _, err := c.Verify(x509.VerifyOptions{Roots: pool, DNSName: name}); err != nil {
			t.Errorf("%s : %v", name, err)
		}
	}
	if _, err := c.Verify(x509.VerifyOptions{Roots: pool, DNSName: "autre"}); err == nil {
		t.Error("nom non couvert accepté")
	}
}

func TestDatabaseComposeSSL(t *testing.T) {
	for typ, want := range map[string][]string{
		"postgresql": {"ssl=on", "ssl_cert_file=/var/lib/postgresql/certs/server.crt", "target: /var/lib/postgresql/certs"},
		"mysql":      {"--require-secure-transport=1", "--ssl-ca=/etc/mysql/certs/ca.crt"},
		"mariadb":    {"--ssl-key=/etc/mysql/certs/server.key", "healthcheck.sh"},
		"mongodb":    {"--tlsMode=requireTLS", "--tlsCertificateKeyFile=/etc/mongo/certs/server.pem", "--tls --tlsCAFile /etc/mongo/certs/ca.pem"},
		"redis":      {"--tls-port", "/etc/redis/certs/server.crt", "--requirepass"},
		"keydb":      {"--tls-ca-cert-file", "/etc/keydb/certs/ca.crt"},
	} {
		db := &store.Database{ID: "db1", Type: typ, Image: DBTypes[typ].Image, Settings: store.DefaultDatabaseSettings()}
		db.Settings.EnableSSL = true
		out, err := DatabaseCompose(DatabaseSpec{DB: db, Env: map[string]string{"REDIS_PASSWORD": "p"}, Network: "nodra", Dir: "/data/nodra/databases/db1"})
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range append(want, "source: /data/nodra/databases/db1/ssl") {
			if !strings.Contains(out, w) {
				t.Errorf("%s : %q absent\n%s", typ, w, out)
			}
		}
		db.Settings.EnableSSL = false
		out, _ = DatabaseCompose(DatabaseSpec{DB: db, Env: map[string]string{"REDIS_PASSWORD": "p"}, Network: "nodra", Dir: "/x"})
		if strings.Contains(out, "/ssl") || strings.Contains(out, "tls") {
			t.Errorf("%s sans TLS :\n%s", typ, out)
		}
	}
	// Types sans prise en charge : réglage ignoré.
	db := &store.Database{ID: "db2", Type: "clickhouse", Image: DBTypes["clickhouse"].Image, Settings: store.DefaultDatabaseSettings()}
	db.Settings.EnableSSL = true
	if out, _ := DatabaseCompose(DatabaseSpec{DB: db, Network: "nodra", Dir: "/x"}); strings.Contains(out, "/ssl") {
		t.Errorf("clickhouse :\n%s", out)
	}
}

func TestConnectionURLSSL(t *testing.T) {
	db := &store.Database{Type: "postgresql", Username: "u", DatabaseName: "app", Settings: store.DatabaseSettings{EnableSSL: true}}
	if u := ConnectionURL(db, "p", "h", 5432); !strings.HasSuffix(u, "?sslmode=require") {
		t.Fatal(u)
	}
	db.Settings.SSLMode = "verify-full"
	if u := ConnectionURL(db, "p", "h", 5432); !strings.HasSuffix(u, "?sslmode=verify-full") {
		t.Fatal(u)
	}
	db = &store.Database{Type: "mongodb", Username: "u", Settings: store.DatabaseSettings{EnableSSL: true}}
	if u := ConnectionURL(db, "p", "h", 27017); !strings.HasSuffix(u, "directConnection=true&tls=true") {
		t.Fatal(u)
	}
	if !ValidSSLMode("postgresql", "verify-ca") || ValidSSLMode("postgresql", "requireTLS") || ValidSSLMode("clickhouse", "") || !ValidSSLMode("redis", "") {
		t.Fatal("modes")
	}
}
