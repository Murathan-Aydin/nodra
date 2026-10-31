package servers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestLogDrainDocker lance la configuration Fluent Bit générée sur le Docker local (NODRA_DOCKER_LOGDRAIN=1) et
// vérifie qu'un conteneur au pilote fluentd y arrive avec ses métadonnées.
func TestLogDrainDocker(t *testing.T) {
	if os.Getenv("NODRA_DOCKER_LOGDRAIN") != "1" {
		t.Skip("NODRA_DOCKER_LOGDRAIN=1 non défini")
	}
	dir := t.TempDir()
	header := LogDrainFiles("newrelic", "srv test", LogDrainConfig{LicenseKey: "x", BaseURI: "https://log-api.newrelic.com/log/v1"})
	conf := strings.Split(header["fluent-bit.conf"], "[OUTPUT]")[0] + "[OUTPUT]\n    Name  stdout\n    Match *\n"
	for name, content := range LogDrainFiles("custom", "srv test", LogDrainConfig{CustomConfig: conf}) {
		content = strings.ReplaceAll(content, "nodra-log-drain", "nodra-log-drain-test")
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sh := func(script string) string {
		out, _ := exec.Command("sh", "-c", script).CombinedOutput()
		return string(out)
	}
	compose := "docker compose -f " + filepath.Join(dir, "docker-compose.yml")
	t.Cleanup(func() { sh(compose + " down") })
	if out := sh(compose + " up -d"); !strings.Contains(out, "Started") && !strings.Contains(out, "Running") {
		t.Fatalf("démarrage :\n%s", out)
	}
	time.Sleep(3 * time.Second)
	sh("docker run --rm --log-driver fluentd --log-opt fluentd-address=127.0.0.1:24224 --log-opt env=NODRA_RESOURCE_UUID " +
		"-e NODRA_RESOURCE_UUID=res-42 alpine:3 echo 'bonjour du conteneur'")
	deadline := time.Now().Add(20 * time.Second)
	for {
		logs := sh("docker logs nodra-log-drain-test")
		if strings.Contains(logs, "bonjour du conteneur") {
			if !strings.Contains(logs, `"nodra.resource_id"=>"res-42"`) || !strings.Contains(logs, `"nodra.server_name"=>"srv-test"`) {
				t.Fatalf("métadonnées absentes :\n%s", logs)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("log non reçu :\n%s", logs)
		}
		time.Sleep(time.Second)
	}
}
