package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Log drains : un conteneur Fluent Bit (nodra-log-drain) reçoit sur 127.0.0.1:24224 les logs des
// conteneurs qui l'activent (pilote fluentd) et les envoie à New Relic, Axiom ou une sortie personnalisée.
const (
	LogDrainDir       = "/data/nodra/log-drain"
	TaskLogDrainStart = "log_drain_start"
	TaskLogDrainStop  = "log_drain_stop"
	FluentBitImage    = "cr.fluentbit.io/fluent/fluent-bit:4.0"
)

var LogDrainTypes = []string{"newrelic", "axiom", "custom"}

func init() {
	handlers[TaskLogDrainStart] = logDrainStart
	handlers[TaskLogDrainStop] = logDrainStop
}

// LogDrainConfig : réglages selon le type (secrets chiffrés avec le reste de la configuration).
type LogDrainConfig struct {
	LicenseKey    string `json:"license_key,omitempty"` // New Relic
	BaseURI       string `json:"base_uri,omitempty"`    // New Relic : https://log-api.newrelic.com/log/v1 (EU : log-api.eu…)
	AxiomDomain   string `json:"axiom_domain,omitempty"`
	AxiomDataset  string `json:"axiom_dataset,omitempty"`
	AxiomAPIKey   string `json:"axiom_api_key,omitempty"`
	CustomConfig  string `json:"custom_config,omitempty"`  // fluent-bit.conf complet
	CustomParsers string `json:"custom_parsers,omitempty"` // parsers.conf
}

const masked = "********"

func (c LogDrainConfig) Redacted() LogDrainConfig {
	for _, p := range []*string{&c.LicenseKey, &c.AxiomAPIKey} {
		if *p != "" {
			*p = masked
		}
	}
	return c
}

// Merge applique l'entrée ; un secret masqué conserve l'ancienne valeur.
func (c *LogDrainConfig) Merge(in LogDrainConfig) {
	keep := func(dst *string, v string) {
		if v != masked {
			*dst = v
		}
	}
	keep(&c.LicenseKey, in.LicenseKey)
	keep(&c.AxiomAPIKey, in.AxiomAPIKey)
	c.BaseURI, c.AxiomDomain, c.AxiomDataset, c.CustomConfig, c.CustomParsers = in.BaseURI, in.AxiomDomain, in.AxiomDataset,
		in.CustomConfig, in.CustomParsers
}

var (
	singleToken = regexp.MustCompile(`^[A-Za-z0-9._:/@+-]{1,512}$`)
	// Directives refusées dans une configuration personnalisée : inclusion de fichiers et exécution de commandes.
	forbiddenFluent = regexp.MustCompile(`(?im)^\s*(@include|@set\b|name\s+(exec|exec_wasi|lua)\b|script\s|call\s)`)
)

// Validate vérifie la configuration (valeurs sur un seul jeton : Fluent Bit lit ligne par ligne).
func (c LogDrainConfig) Validate(typ string) error {
	switch typ {
	case "newrelic":
		u, err := url.Parse(c.BaseURI)
		if !singleToken.MatchString(c.LicenseKey) || err != nil || u.Scheme != "https" || u.Host == "" || !singleToken.MatchString(c.BaseURI) {
			return errors.New("license_key et base_uri (https://log-api.newrelic.com/log/v1) requis")
		}
	case "axiom":
		if !singleToken.MatchString(c.AxiomDataset) || !singleToken.MatchString(c.AxiomAPIKey) ||
			(c.AxiomDomain != "" && !singleToken.MatchString(c.AxiomDomain)) {
			return errors.New("axiom_dataset et axiom_api_key requis")
		}
	case "custom":
		if strings.TrimSpace(c.CustomConfig) == "" || len(c.CustomConfig) > 64<<10 || len(c.CustomParsers) > 64<<10 {
			return errors.New("custom_config requis (64 Ko max.)")
		}
		if forbiddenFluent.MatchString(c.CustomConfig) || forbiddenFluent.MatchString(c.CustomParsers) {
			return errors.New("custom_config : @INCLUDE, @SET et les extensions exec/lua sont refusés")
		}
	default:
		return errors.New("type : newrelic, axiom ou custom")
	}
	return nil
}

const fluentHeader = `[SERVICE]
    Flush     5
    Daemon    off
    Log_Level warn
    Parsers_File  parsers.conf

[INPUT]
    Name              forward
    Listen            0.0.0.0
    Port              24224
    Buffer_Chunk_Size 1M
    Buffer_Max_Size   6M

[FILTER]
    Name   modify
    Match  *
    Set    nodra.server_name %s
    Rename NODRA_RESOURCE_UUID nodra.resource_id
    Rename NODRA_CONTAINER_NAME nodra.container_name
`

// LogDrainFiles : fichiers du log drain (chemin relatif → contenu) ; .env porte les secrets.
func LogDrainFiles(typ, serverName string, c LogDrainConfig) map[string]string {
	name := regexp.MustCompile(`[^A-Za-z0-9._-]+`).ReplaceAllString(strings.TrimSpace(serverName), "-")
	if name == "" {
		name = "serveur"
	}
	parsers := "[PARSER]\n    Name        empty_line_skipper\n    Format      regex\n    Regex       /^(?!\\s*$).+/\n"
	conf, env := "", ""
	switch typ {
	case "newrelic":
		conf = fmt.Sprintf(fluentHeader, name) + "\n[OUTPUT]\n    Name        nrlogs\n    Match       *\n    license_key ${LICENSE_KEY}\n    base_uri    ${BASE_URI}\n"
		env = "LICENSE_KEY=" + c.LicenseKey + "\nBASE_URI=" + c.BaseURI + "\n"
	case "axiom":
		domain := c.AxiomDomain
		if domain == "" {
			domain = "api.axiom.co"
		}
		conf = fmt.Sprintf(fluentHeader, name) + "\n[OUTPUT]\n    Name             http\n    Match            *\n    Host             ${AXIOM_DOMAIN}\n" +
			"    Port             443\n    URI              /v1/datasets/${AXIOM_DATASET_NAME}/ingest\n" +
			"    Header           Authorization Bearer ${AXIOM_API_KEY}\n    compress         gzip\n    format           json\n" +
			"    json_date_key    _time\n    json_date_format iso8601\n    tls              On\n"
		env = "AXIOM_DOMAIN=" + domain + "\nAXIOM_DATASET_NAME=" + c.AxiomDataset + "\nAXIOM_API_KEY=" + c.AxiomAPIKey + "\n"
	case "custom":
		conf = c.CustomConfig
		if c.CustomParsers != "" {
			parsers = c.CustomParsers
		}
	}
	compose := `name: nodra-log-drain
services:
  fluent-bit:
    image: ` + FluentBitImage + `
    container_name: nodra-log-drain
    command: ["-c", "/fluent-bit/etc/fluent-bit.conf"]
    env_file: [.env]
    volumes:
      - ./fluent-bit.conf:/fluent-bit/etc/fluent-bit.conf:ro
      - ./parsers.conf:/fluent-bit/etc/parsers.conf:ro
    ports:
      - 127.0.0.1:24224:24224
    labels:
      - nodra.managed=true
      - nodra.log_drain=true
    restart: unless-stopped
`
	return map[string]string{"fluent-bit.conf": conf, "parsers.conf": parsers, ".env": env, "docker-compose.yml": compose}
}

func logDrainStart(ctx context.Context, r *taskRun) error {
	d, err := r.svc.Store.LogDrain(ctx, r.server.ID)
	if err != nil {
		return fmt.Errorf("aucun log drain configuré : %w", err)
	}
	raw, err := r.svc.Box.Decrypt(d.ConfigEnc)
	if err != nil {
		return err
	}
	var c LogDrainConfig
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return err
	}
	files := map[string]string{}
	for name, content := range LogDrainFiles(d.Type, r.server.Name, c) {
		files[LogDrainDir+"/"+name] = content
	}
	r.out.logf("Configuration du log drain (%s) dans %s\n", d.Type, LogDrainDir)
	if err := r.script(ctx, "écriture de la configuration du log drain", writeFilesScript(files, []string{LogDrainDir})+
		"$SUDO chmod 600 "+LogDrainDir+"/.env\n"); err != nil {
		return err
	}
	compose := "docker compose -f " + LogDrainDir + "/docker-compose.yml"
	if err := r.exec(ctx, compose+" pull --quiet"); err != nil {
		return err
	}
	return r.exec(ctx, "docker rm -f nodra-log-drain >/dev/null 2>&1; "+compose+" up -d")
}

func logDrainStop(ctx context.Context, r *taskRun) error {
	return r.exec(ctx, r.sudo("docker rm -f nodra-log-drain >/dev/null 2>&1; rm -rf "+LogDrainDir))
}

// DecodeLogDrain : configuration déchiffrée d'un log drain.
func DecodeLogDrain(raw string) (LogDrainConfig, error) {
	var c LogDrainConfig
	err := json.Unmarshal([]byte(raw), &c)
	return c, err
}
