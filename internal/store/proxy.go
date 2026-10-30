package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) UpdateProxy(ctx context.Context, serverID string, p ProxyInfo) error {
	_, err := s.Pool.Exec(ctx, `UPDATE servers SET proxy_type = $2, proxy_compose = $3, proxy_acme_email = $4, updated_at = now()
		WHERE id = $1`, serverID, p.Type, p.Compose, p.AcmeEmail)
	return err
}

func (s *Store) SetProxyStatus(ctx context.Context, serverID, status string) error {
	var team, was string
	err := s.Pool.QueryRow(ctx, `UPDATE servers s SET proxy_status = $2 FROM (SELECT id, proxy_status FROM servers WHERE id = $1) o
		WHERE s.id = o.id RETURNING s.team_id::text, o.proxy_status`, serverID, status).Scan(&team, &was)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err == nil && was != status {
		s.Emit(ctx, Event{Team: team, Kind: EventProxyStatus, ID: serverID, Server: serverID, Status: status})
	}
	return err
}

type ProxyConfig struct {
	ID        string    `json:"id"`
	ServerID  string    `json:"server_id"`
	Name      string    `json:"name"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) ListProxyConfigs(ctx context.Context, serverID string) ([]*ProxyConfig, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, server_id::text, name, content, updated_at FROM proxy_configs
		WHERE server_id = $1 ORDER BY name`, serverID)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(row pgx.Row) (*ProxyConfig, error) {
		c := &ProxyConfig{}
		return c, row.Scan(&c.ID, &c.ServerID, &c.Name, &c.Content, &c.UpdatedAt)
	})
}

func (s *Store) UpsertProxyConfig(ctx context.Context, serverID, name, content string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO proxy_configs (server_id, name, content) VALUES ($1, $2, $3)
		ON CONFLICT (server_id, name) DO UPDATE SET content = EXCLUDED.content, updated_at = now()`, serverID, name, content)
	return err
}

func (s *Store) DeleteProxyConfig(ctx context.Context, serverID, name string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM proxy_configs WHERE server_id = $1 AND name = $2`, serverID, name)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

type Certificate struct {
	ID            string    `json:"id"`
	ServerID      string    `json:"server_id"`
	Name          string    `json:"name"`
	Domains       []string  `json:"domains"`
	Certificate   string    `json:"certificate"`
	PrivateKeyEnc string    `json:"-"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
}

const certCols = `id::text, server_id::text, name, domains, certificate, private_key_enc, expires_at, created_at`

func scanCert(row pgx.Row) (*Certificate, error) {
	c := &Certificate{}
	err := row.Scan(&c.ID, &c.ServerID, &c.Name, &c.Domains, &c.Certificate, &c.PrivateKeyEnc, &c.ExpiresAt, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *Store) ListCertificates(ctx context.Context, serverID string) ([]*Certificate, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+certCols+` FROM ssl_certificates WHERE server_id = $1 ORDER BY name`, serverID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanCert)
}

func (s *Store) CreateCertificate(ctx context.Context, c *Certificate) (*Certificate, error) {
	r, err := scanCert(s.Pool.QueryRow(ctx, `INSERT INTO ssl_certificates (server_id, name, domains, certificate, private_key_enc, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+certCols, c.ServerID, c.Name, c.Domains, c.Certificate, c.PrivateKeyEnc, c.ExpiresAt))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return r, err
}

func (s *Store) DeleteCertificate(ctx context.Context, serverID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM ssl_certificates WHERE server_id = $1 AND id = $2`, serverID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
