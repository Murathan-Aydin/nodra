// Package config charge la configuration de Nodra depuis les variables d'environnement.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Addr        string // adresse d'écoute HTTP
	DatabaseURL string // DSN PostgreSQL
	AppKey      []byte // clé AES-256 (32 octets) pour chiffrer les secrets
	Env         string // "production" ou "development"
	// ServerDataDir : dossier de travail de Nodra sur les serveurs gérés (applications, bases, services,
	// sauvegardes). /data/nodra par défaut ; le proxy reste dans /data/nodra/proxy.
	ServerDataDir string
	// PublicURL : URL publique de l'instance (liens des e-mails et notifications) si le domaine n'est pas réglé.
	PublicURL string
	// TrustProxy : faire confiance à X-Forwarded-For / X-Real-IP (uniquement derrière un proxy maîtrisé).
	TrustProxy bool
}

func Load() (*Config, error) {
	c := &Config{
		Addr:          getenv("NODRA_ADDR", ":8000"),
		DatabaseURL:   os.Getenv("NODRA_DATABASE_URL"),
		Env:           getenv("NODRA_ENV", "production"),
		TrustProxy:    os.Getenv("NODRA_TRUST_PROXY") == "true",
		PublicURL:     strings.TrimSuffix(os.Getenv("NODRA_PUBLIC_URL"), "/"),
		ServerDataDir: strings.TrimSuffix(getenv("NODRA_SERVER_DATA_DIR", "/data/nodra"), "/"),
	}
	if c.DatabaseURL == "" {
		return nil, errors.New("NODRA_DATABASE_URL est requis")
	}
	raw := os.Getenv("NODRA_APP_KEY")
	if raw == "" {
		return nil, errors.New("NODRA_APP_KEY est requis (32 octets en base64, ex. `openssl rand -base64 32`)")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("NODRA_APP_KEY doit être 32 octets encodés en base64")
	}
	c.AppKey = key
	if !strings.HasPrefix(c.ServerDataDir, "/") || strings.Contains(c.ServerDataDir, "..") {
		return nil, errors.New("NODRA_SERVER_DATA_DIR doit être un chemin absolu")
	}
	return c, nil
}

func (c *Config) IsDev() bool { return c.Env == "development" }

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
