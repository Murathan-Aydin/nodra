package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Session struct {
	TokenHash  string    `json:"-"`
	ID         string    `json:"id"` // 12 premiers caractères du hash, pour l'affichage/révocation
	UserID     string    `json:"-"`
	MFAPending bool      `json:"-"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

const sessionCols = `token_hash, left(token_hash, 12), user_id::text, mfa_pending, ip, user_agent, expires_at, last_seen_at, created_at`

func scanSession(row pgx.Row) (*Session, error) {
	s := &Session{}
	err := row.Scan(&s.TokenHash, &s.ID, &s.UserID, &s.MFAPending, &s.IP, &s.UserAgent, &s.ExpiresAt, &s.LastSeenAt, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

func (s *Store) CreateSession(ctx context.Context, sess *Session) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO sessions (token_hash, user_id, mfa_pending, ip, user_agent, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)`, sess.TokenHash, sess.UserID, sess.MFAPending, sess.IP, sess.UserAgent, sess.ExpiresAt)
	return err
}

// SessionByHash renvoie une session non expirée et prolonge son activité.
func (s *Store) SessionByHash(ctx context.Context, hash string) (*Session, error) {
	return scanSession(s.Pool.QueryRow(ctx, `UPDATE sessions SET last_seen_at = now()
		WHERE token_hash = $1 AND expires_at > now() RETURNING `+sessionCols, hash))
}

func (s *Store) CompleteMFA(ctx context.Context, hash string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE sessions SET mfa_pending = false WHERE token_hash = $1`, hash)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, hash string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hash)
	return err
}

func (s *Store) ListSessions(ctx context.Context, userID string) ([]*Session, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+sessionCols+` FROM sessions
		WHERE user_id = $1 AND expires_at > now() AND NOT mfa_pending ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSession)
}

// DeleteSessionByID révoque une session de l'utilisateur à partir de son identifiant court.
func (s *Store) DeleteSessionByID(ctx context.Context, userID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND left(token_hash, 12) = $2`, userID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteExpiredSessions(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return err
}

func collect[T any](rows pgx.Rows, scan func(pgx.Row) (*T, error)) ([]*T, error) {
	defer rows.Close()
	out := []*T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
