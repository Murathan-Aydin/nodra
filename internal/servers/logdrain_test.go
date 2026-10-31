package servers

import (
	"strings"
	"testing"
)

func TestLogDrainConfig(t *testing.T) {
	nr := LogDrainConfig{LicenseKey: "NRAK-123", BaseURI: "https://log-api.eu.newrelic.com/log/v1"}
	if err := nr.Validate("newrelic"); err != nil {
		t.Fatal(err)
	}
	files := LogDrainFiles("newrelic", "prod 1\n[OUTPUT]", nr)
	if !strings.Contains(files["fluent-bit.conf"], "Name        nrlogs") || !strings.Contains(files[".env"], "LICENSE_KEY=NRAK-123") ||
		!strings.Contains(files["fluent-bit.conf"], "nodra.server_name prod-1-OUTPUT-") || strings.Contains(files["fluent-bit.conf"], "NRAK-123") {
		t.Fatalf("new relic :\n%s\n%s", files["fluent-bit.conf"], files[".env"])
	}
	if !strings.Contains(files["docker-compose.yml"], "127.0.0.1:24224:24224") {
		t.Fatal("port local attendu")
	}
	ax := LogDrainConfig{AxiomDataset: "logs", AxiomAPIKey: "xaat-1"}
	if err := ax.Validate("axiom"); err != nil {
		t.Fatal(err)
	}
	if f := LogDrainFiles("axiom", "s", ax); !strings.Contains(f[".env"], "AXIOM_DOMAIN=api.axiom.co") || !strings.Contains(f["fluent-bit.conf"], "/v1/datasets/${AXIOM_DATASET_NAME}/ingest") {
		t.Fatalf("axiom : %v", f)
	}
	for name, c := range map[string]struct {
		typ string
		cfg LogDrainConfig
	}{
		"clé multiligne":   {"newrelic", LogDrainConfig{LicenseKey: "a\nb", BaseURI: "https://x.example/log"}},
		"http":             {"newrelic", LogDrainConfig{LicenseKey: "a", BaseURI: "http://x.example/log"}},
		"dataset manquant": {"axiom", LogDrainConfig{AxiomAPIKey: "k"}},
		"include":          {"custom", LogDrainConfig{CustomConfig: "@INCLUDE /etc/passwd"}},
		"exec":             {"custom", LogDrainConfig{CustomConfig: "[INPUT]\n    Name exec\n    Command id\n"}},
		"vide":             {"custom", LogDrainConfig{}},
		"type":             {"loki", LogDrainConfig{}},
	} {
		if c.cfg.Validate(c.typ) == nil {
			t.Errorf("%s : accepté", name)
		}
	}
	if (LogDrainConfig{CustomConfig: "[INPUT]\n    Name forward\n[OUTPUT]\n    Name stdout\n    Match *\n"}).Validate("custom") != nil {
		t.Error("configuration personnalisée valide refusée")
	}
	r := nr.Redacted()
	if r.LicenseKey != "********" || r.BaseURI != nr.BaseURI {
		t.Fatalf("masquage : %+v", r)
	}
	nr.Merge(LogDrainConfig{LicenseKey: "********", BaseURI: "https://log-api.newrelic.com/log/v1"})
	if nr.LicenseKey != "NRAK-123" || nr.BaseURI != "https://log-api.newrelic.com/log/v1" {
		t.Fatalf("fusion : %+v", nr)
	}
}
