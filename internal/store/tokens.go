package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type APIToken struct {
	ID         string     `json:"id"`
	UserID     string     `json:"-"`
	Name       string     `json:"name"`
	TokenHash  string     `json:"-"`
	Scopes     []string   `json:"scopes"`
	ExpiresAt  *time.Time `json:"expires_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

const tokenCols = `id::text, user_id::text, name, token_hash, scopes, expires_at, last_used_at, created_at`

func scanToken(row pgx.Row) (*APIToken, error) {
	t := &APIToken{}
	err := row.Scan(&t.ID, &t.UserID, &t.Name, &t.TokenHash, &t.Scopes, &t.ExpiresAt, &t.LastUsedAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Store) CreateAPIToken(ctx context.Context, t *APIToken) (*APIToken, error) {
	return scanToken(s.Pool.QueryRow(ctx, `INSERT INTO api_tokens (user_id, name, token_hash, scopes, expires_at)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+tokenCols, t.UserID, t.Name, t.TokenHash, t.Scopes, t.ExpiresAt))
}

// APITokenByHash renvoie un jeton valide et met à jour sa date de dernière utilisation.
func (s *Store) APITokenByHash(ctx context.Context, hash string) (*APIToken, error) {
	return scanToken(s.Pool.QueryRow(ctx, `UPDATE api_tokens SET last_used_at = now()
		WHERE token_hash = $1 AND (expires_at IS NULL OR expires_at > now()) RETURNING `+tokenCols, hash))
}

func (s *Store) ListAPITokens(ctx context.Context, userID string) ([]*APIToken, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+tokenCols+` FROM api_tokens WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanToken)
}

func (s *Store) DeleteAPIToken(ctx context.Context, userID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM api_tokens WHERE user_id = $1 AND id::text = $2`, userID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) CreatePasswordReset(ctx context.Context, userID, hash string, expires time.Time) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM password_resets WHERE user_id = $1`, userID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO password_resets (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`, hash, userID, expires)
		return err
	})
}

// ConsumePasswordReset supprime le jeton et renvoie l'utilisateur associé s'il est valide.
func (s *Store) ConsumePasswordReset(ctx context.Context, hash string) (string, error) {
	var uid string
	err := s.Pool.QueryRow(ctx, `DELETE FROM password_resets WHERE token_hash = $1 AND expires_at > now()
		RETURNING user_id::text`, hash).Scan(&uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return uid, err
}
