package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type NotificationChannel struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"team_id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	ConfigEnc string    `json:"-"`
	Events    []string  `json:"events"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

const channelCols = `id::text, team_id::text, type, name, config_enc, events, enabled, created_at`

func scanChannel(row pgx.Row) (*NotificationChannel, error) {
	c := &NotificationChannel{}
	err := row.Scan(&c.ID, &c.TeamID, &c.Type, &c.Name, &c.ConfigEnc, &c.Events, &c.Enabled, &c.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

func (s *Store) CreateChannel(ctx context.Context, c *NotificationChannel) (*NotificationChannel, error) {
	return scanChannel(s.Pool.QueryRow(ctx, `INSERT INTO notification_channels (team_id, type, name, config_enc, events, enabled)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+channelCols, c.TeamID, c.Type, c.Name, c.ConfigEnc, c.Events, c.Enabled))
}

func (s *Store) UpdateChannel(ctx context.Context, c *NotificationChannel) error {
	_, err := s.Pool.Exec(ctx, `UPDATE notification_channels SET name = $2, config_enc = $3, events = $4, enabled = $5, updated_at = now()
		WHERE id = $1`, c.ID, c.Name, c.ConfigEnc, c.Events, c.Enabled)
	return err
}

func (s *Store) ChannelByID(ctx context.Context, teamID, id string) (*NotificationChannel, error) {
	return scanChannel(s.Pool.QueryRow(ctx, `SELECT `+channelCols+` FROM notification_channels WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) ListChannels(ctx context.Context, teamID string) ([]*NotificationChannel, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+channelCols+` FROM notification_channels WHERE team_id = $1 ORDER BY lower(name)`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanChannel)
}

// SubscribedChannels : canaux actifs de l'équipe (et de ses équipes parentes) abonnés à l'événement.
func (s *Store) SubscribedChannels(ctx context.Context, teamID, event string) ([]*NotificationChannel, error) {
	chain, err := s.TeamChain(ctx, teamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+channelCols+` FROM notification_channels WHERE team_id = ANY($1::uuid[]) AND enabled
		AND ($2 = 'test' OR $2 = ANY(events))`, chain, event)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanChannel)
}

func (s *Store) DeleteChannel(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM notification_channels WHERE team_id = $1 AND id = $2`, teamID, id)
	return err
}

type NotificationDelivery struct {
	ID        int64     `json:"id"`
	ChannelID *string   `json:"channel_id"`
	Event     string    `json:"event"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Store) RecordDelivery(ctx context.Context, teamID, channelID, event, title, status, errMsg string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO notification_deliveries (team_id, channel_id, event, title, status, error)
		VALUES ($1, $2, $3, $4, $5, $6)`, teamID, channelID, event, title, status, errMsg)
	return err
}

func (s *Store) ListDeliveries(ctx context.Context, teamID string, limit int) ([]*NotificationDelivery, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, channel_id::text, event, title, status, error, created_at FROM notification_deliveries
		WHERE team_id = $1 ORDER BY id DESC LIMIT $2`, teamID, limit)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(row pgx.Row) (*NotificationDelivery, error) {
		d := &NotificationDelivery{}
		return d, row.Scan(&d.ID, &d.ChannelID, &d.Event, &d.Title, &d.Status, &d.Error, &d.CreatedAt)
	})
}

// Throttle renvoie true (et enregistre l'envoi) si aucune notification de clé key n'a été envoyée depuis every.
func (s *Store) Throttle(ctx context.Context, key string, every time.Duration) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `INSERT INTO notification_throttles (key, sent_at) VALUES ($1, now())
		ON CONFLICT (key) DO UPDATE SET sent_at = now() WHERE notification_throttles.sent_at < now() - $2::interval`,
		key, every.String())
	return tag.RowsAffected() == 1, err
}

func (s *Store) ClearThrottle(ctx context.Context, key string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM notification_throttles WHERE key = $1`, key)
	return err
}

func (s *Store) PurgeDeliveries(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM notification_deliveries WHERE created_at < now() - interval '30 days'`)
	return err
}

// --- E-mail de l'instance ---

func (s *Store) MailConfigEnc(ctx context.Context) (string, error) {
	var enc string
	err := s.Pool.QueryRow(ctx, `SELECT mail_config_enc FROM instance_settings WHERE id = 1`).Scan(&enc)
	return enc, err
}

func (s *Store) SetMailConfigEnc(ctx context.Context, enc string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE instance_settings SET mail_config_enc = $1, updated_at = now() WHERE id = 1`, enc)
	return err
}

// --- Vérification d'e-mail ---

func (s *Store) CreateEmailVerification(ctx context.Context, userID, hash string, expires time.Time) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM email_verifications WHERE user_id = $1`, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO email_verifications (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, hash, userID, expires)
		return err
	})
}

// ConsumeEmailVerification marque l'adresse comme vérifiée ; renvoie l'utilisateur.
func (s *Store) ConsumeEmailVerification(ctx context.Context, hash string) (string, error) {
	var uid string
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `DELETE FROM email_verifications WHERE token_hash = $1 AND expires_at > now() RETURNING user_id::text`,
			hash).Scan(&uid); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE users SET email_verified_at = now(), updated_at = now() WHERE id = $1`, uid)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return uid, err
}

// --- Échéances ---

// ExpiringAPITokens : jetons expirant dans la fenêtre donnée (avec l'équipe et l'e-mail du propriétaire).
func (s *Store) ExpiringAPITokens(ctx context.Context, within time.Duration) ([]*APIToken, []string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+prefixed(tokenCols, "t.")+`, u.email FROM api_tokens t JOIN users u ON u.id = t.user_id
		WHERE t.expires_at IS NOT NULL AND t.expires_at > now() AND t.expires_at < now() + $1::interval AND t.team_id IS NOT NULL`,
		within.String())
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var list []*APIToken
	var emails []string
	for rows.Next() {
		t := &APIToken{}
		var email string
		if err := rows.Scan(&t.ID, &t.UserID, &t.TeamID, &t.Name, &t.TokenHash, &t.Scopes, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt, &email); err != nil {
			return nil, nil, err
		}
		list, emails = append(list, t), append(emails, email)
	}
	return list, emails, rows.Err()
}

// ExpiringCertificates : certificats personnalisés expirant dans la fenêtre donnée (avec l'équipe du serveur).
func (s *Store) ExpiringCertificates(ctx context.Context, within time.Duration) ([]*Certificate, []string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+prefixed(certCols, "c.")+`, sv.team_id::text FROM ssl_certificates c
		JOIN servers sv ON sv.id = c.server_id WHERE c.expires_at < now() + $1::interval`, within.String())
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var list []*Certificate
	var teams []string
	for rows.Next() {
		c := &Certificate{}
		var team string
		if err := rows.Scan(&c.ID, &c.ServerID, &c.Name, &c.Domains, &c.Certificate, &c.PrivateKeyEnc, &c.ExpiresAt, &c.CreatedAt, &team); err != nil {
			return nil, nil, err
		}
		list, teams = append(list, c), append(teams, team)
	}
	return list, teams, rows.Err()
}

// ResourceName renvoie le nom et le type (application, database, service) d'une ressource.
func (s *Store) ResourceName(ctx context.Context, id string) (name, typ string, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT name, type FROM resources WHERE id = $1`, id).Scan(&name, &typ)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return
}
