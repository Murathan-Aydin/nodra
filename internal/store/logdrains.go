package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// LogDrain : envoi des logs des conteneurs d'un serveur (Fluent Bit).
type LogDrain struct {
	ServerID  string    `json:"server_id"`
	Type      string    `json:"type"` // newrelic | axiom | custom
	ConfigEnc string    `json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) LogDrain(ctx context.Context, serverID string) (*LogDrain, error) {
	d := &LogDrain{}
	err := s.Pool.QueryRow(ctx, `SELECT server_id::text, type, config_enc, updated_at FROM server_log_drains WHERE server_id = $1`, serverID).
		Scan(&d.ServerID, &d.Type, &d.ConfigEnc, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

func (s *Store) UpsertLogDrain(ctx context.Context, d *LogDrain) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO server_log_drains (server_id, type, config_enc) VALUES ($1, $2, $3)
		ON CONFLICT (server_id) DO UPDATE SET type = EXCLUDED.type, config_enc = EXCLUDED.config_enc, updated_at = now()`,
		d.ServerID, d.Type, d.ConfigEnc)
	return err
}

func (s *Store) DeleteLogDrain(ctx context.Context, serverID string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM server_log_drains WHERE server_id = $1`, serverID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// HasLogDrain indique si le serveur d'une ressource a un log drain.
func (s *Store) HasLogDrain(ctx context.Context, serverID string) bool {
	var ok bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM server_log_drains WHERE server_id = $1)`, serverID).Scan(&ok)
	return ok
}

// SetServiceLogDrain active ou non l'envoi des logs d'un service.
func (s *Store) SetServiceLogDrain(ctx context.Context, id string, enabled bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE services SET log_drain_enabled = $2, updated_at = now() WHERE id = $1`, id, enabled)
	return err
}
