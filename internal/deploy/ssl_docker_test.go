package deploy

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

// TestDatabaseSSLDocker démarre de vraies bases avec TLS sur le Docker local (NODRA_DOCKER_SSL=1) et
// vérifie une connexion chiffrée depuis le conteneur (certificat vérifié par l'autorité générée).
func TestDatabaseSSLDocker(t *testing.T) {
	if os.Getenv("NODRA_DOCKER_SSL") != "1" {
		t.Skip("NODRA_DOCKER_SSL=1 non défini")
	}
	sh := func(script string) (string, error) {
		out, err := exec.Command("sh", "-c", script).CombinedOutput()
		return string(out), err
	}
	const network = "nodra-ssl-test"
	_, _ = sh("docker network create " + network)
	t.Cleanup(func() { _, _ = sh("docker network rm " + network) })
	caCert, caKey, err := GenerateCA("Nodra CA test")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		typ, image, check, want string
	}{
		{"postgresql", "", `psql "host=localhost user=nodra dbname=app sslmode=verify-full sslrootcert=/var/lib/postgresql/certs/ca.crt" -tAc "SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()"`, "t"},
		{"mariadb", "", `mariadb --ssl-verify-server-cert --ssl-ca=/etc/mysql/certs/ca.crt -h localhost --protocol=tcp -unodra -ppass123 -NBe "SHOW STATUS LIKE 'Ssl_cipher'"`, "Ssl_cipher\tTLS"},
		{"mysql", "mysql:8.4", `mysql --ssl-mode=VERIFY_IDENTITY --ssl-ca=/etc/mysql/certs/ca.crt -h localhost --protocol=tcp -unodra -ppass123 -NBe "SHOW STATUS LIKE 'Ssl_cipher'"`, "Ssl_cipher\tTLS"},
		{"redis", "", `redis-cli --tls --cacert /etc/redis/certs/ca.crt -h localhost -p 6380 -a pass123 --no-auth-warning ping`, "PONG"},
	} {
		t.Run(c.typ, func(t *testing.T) {
			dir := t.TempDir()
			db := &store.Database{ID: "nodra-ssl-" + c.typ, Type: c.typ, Image: DBTypes[c.typ].Image, Username: "nodra", DatabaseName: "app",
				Settings: store.DefaultDatabaseSettings()}
			if c.image != "" {
				db.Image = c.image
			}
			db.Settings.EnableSSL = true
			env := DatabaseEnv(db, "pass123", "rootpass123", nil)
			compose, err := DatabaseCompose(DatabaseSpec{DB: db, Env: env, Network: network, Dir: dir})
			if err != nil {
				t.Fatal(err)
			}
			cert, key, err := IssueCert(caCert, caKey, db.ID, []string{"localhost", "127.0.0.1"})
			if err != nil {
				t.Fatal(err)
			}
			keyMode := os.FileMode(0o600)
			if !SSLTypes[c.typ].Image { // propriétaire posé sur l'hôte en production (chown 999) : simulé par la lecture pour tous
				keyMode = 0o644
			}
			_ = os.MkdirAll(filepath.Join(dir, "ssl"), 0o755)
			for name, content := range map[string]string{"ca.crt": caCert, "ca.pem": caCert, "server.crt": cert, "server.key": key, "server.pem": key + cert} {
				mode := os.FileMode(0o644)
				if strings.HasPrefix(name, "server.k") || name == "server.pem" {
					mode = keyMode
				}
				if err := os.WriteFile(filepath.Join(dir, "ssl", name), []byte(content), mode); err != nil {
					t.Fatal(err)
				}
			}
			file := filepath.Join(dir, "docker-compose.yml")
			if err := os.WriteFile(file, []byte(compose), 0o600); err != nil {
				t.Fatal(err)
			}
			base := "docker compose -p " + db.ID + " -f " + file
			t.Cleanup(func() { _, _ = sh(base + " down -v") })
			if SSLTypes[c.typ].Image {
				if out, err := sh(sslOwnership(db, base, dir)); err != nil {
					t.Fatalf("chown : %v\n%s", err, out)
				}
			}
			if out, err := sh(base + " up -d"); err != nil {
				t.Fatalf("up : %v\n%s", err, out)
			}
			deadline := time.Now().Add(3 * time.Minute)
			for {
				status, _ := sh("docker inspect -f '{{.State.Health.Status}}' " + db.ID)
				if strings.TrimSpace(status) == "healthy" {
					break
				}
				if time.Now().After(deadline) {
					logs, _ := sh("docker logs --tail 30 " + db.ID)
					t.Fatalf("base non saine (%s)\n%s", status, logs)
				}
				time.Sleep(2 * time.Second)
			}
			out, err := sh("docker exec -e PGPASSWORD=pass123 " + db.ID + " sh -c '" + strings.ReplaceAll(c.check, "'", `'"'"'`) + "'")
			if err != nil || !strings.Contains(out, c.want) {
				t.Fatalf("connexion TLS : %v\n%s", err, out)
			}
		})
	}
}
