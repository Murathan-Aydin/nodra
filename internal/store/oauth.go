package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// OAuthProvider : réglages d'un fournisseur de connexion.
type OAuthProvider struct {
	Provider        string    `json:"provider"`
	Enabled         bool      `json:"enabled"`
	ClientID        string    `json:"client_id"`
	ClientSecretEnc string    `json:"-"`
	BaseURL         string    `json:"base_url"`
	Tenant          string    `json:"tenant"`
	UpdatedAt       time.Time `json:"updated_at"`
}

const oauthCols = `provider, enabled, client_id, client_secret_enc, base_url, tenant, updated_at`

func scanOAuth(row pgx.Row) (*OAuthProvider, error) {
	p := &OAuthProvider{}
	err := row.Scan(&p.Provider, &p.Enabled, &p.ClientID, &p.ClientSecretEnc, &p.BaseURL, &p.Tenant, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) ListOAuthProviders(ctx context.Context) ([]*OAuthProvider, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+oauthCols+` FROM oauth_providers ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanOAuth)
}

func (s *Store) OAuthProviderByName(ctx context.Context, provider string) (*OAuthProvider, error) {
	return scanOAuth(s.Pool.QueryRow(ctx, `SELECT `+oauthCols+` FROM oauth_providers WHERE provider = $1`, provider))
}

func (s *Store) UpsertOAuthProvider(ctx context.Context, p *OAuthProvider) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO oauth_providers (provider, enabled, client_id, client_secret_enc, base_url, tenant)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (provider) DO UPDATE SET enabled = EXCLUDED.enabled,
		client_id = EXCLUDED.client_id, client_secret_enc = EXCLUDED.client_secret_enc, base_url = EXCLUDED.base_url,
		tenant = EXCLUDED.tenant, updated_at = now()`, p.Provider, p.Enabled, p.ClientID, p.ClientSecretEnc, p.BaseURL, p.Tenant)
	return err
}

// OAuthState : flux d'autorisation en cours.
type OAuthState struct {
	Provider   string
	Verifier   string
	Redirect   string
	LinkUserID *string
}

func (s *Store) CreateOAuthState(ctx context.Context, hash string, st *OAuthState, ttl time.Duration) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO oauth_states (state_hash, provider, verifier, redirect, link_user_id, expires_at)
		VALUES ($1, $2, $3, $4, $5, now() + $6::interval)`, hash, st.Provider, st.Verifier, st.Redirect, st.LinkUserID,
		ttl.String())
	return err
}

// ConsumeOAuthState lit et supprime un état non expiré (usage unique).
func (s *Store) ConsumeOAuthState(ctx context.Context, hash string) (*OAuthState, error) {
	st := &OAuthState{}
	err := s.Pool.QueryRow(ctx, `DELETE FROM oauth_states WHERE state_hash = $1 AND expires_at > now()
		RETURNING provider, verifier, redirect, link_user_id::text`, hash).Scan(&st.Provider, &st.Verifier, &st.Redirect, &st.LinkUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return st, err
}

func (s *Store) PurgeOAuthStates(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM oauth_states WHERE expires_at < now()`)
	return err
}

// Identity : compte externe lié à un utilisateur.
type Identity struct {
	Provider  string    `json:"provider"`
	Subject   string    `json:"subject"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// IdentityUser : utilisateur lié au compte externe (ErrNotFound sinon).
func (s *Store) IdentityUser(ctx context.Context, provider, subject string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT user_id::text FROM user_identities WHERE provider = $1 AND subject = $2`, provider, subject).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

// LinkIdentity lie un compte externe ; ErrConflict s'il est déjà lié (à cet utilisateur ou à un autre)
// ou si l'utilisateur a déjà un compte chez ce fournisseur.
func (s *Store) LinkIdentity(ctx context.Context, userID, provider, subject, email string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO user_identities (provider, subject, user_id, email) VALUES ($1, $2, $3, $4)`,
		provider, subject, userID, email)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) ListIdentities(ctx context.Context, userID string) ([]*Identity, error) {
	rows, err := s.Pool.Query(ctx, `SELECT provider, subject, email, created_at FROM user_identities WHERE user_id = $1 ORDER BY provider`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(row pgx.Row) (*Identity, error) {
		i := &Identity{}
		return i, row.Scan(&i.Provider, &i.Subject, &i.Email, &i.CreatedAt)
	})
}

func (s *Store) DeleteIdentity(ctx context.Context, userID, provider string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM user_identities WHERE user_id = $1 AND provider = $2`, userID, provider)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// MarkEmailVerified : adresse confirmée par un fournisseur.
func (s *Store) MarkEmailVerified(ctx context.Context, userID string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET email_verified_at = coalesce(email_verified_at, now()) WHERE id = $1`, userID)
	return err
}
