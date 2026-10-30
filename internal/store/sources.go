package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type GitSource struct {
	ID               string     `json:"id"`
	TeamID           string     `json:"team_id"`
	Type             string     `json:"type"`
	Name             string     `json:"name"`
	HTMLURL          string     `json:"html_url"`
	APIURL           string     `json:"api_url"`
	Organization     string     `json:"organization"`
	IsPublic         bool       `json:"is_public"`
	AppID            int64      `json:"app_id"`
	AppSlug          string     `json:"app_slug"`
	ClientID         string     `json:"client_id"`
	ClientSecretEnc  string     `json:"-"`
	WebhookSecretEnc string     `json:"-"`
	PrivateKeyEnc    string     `json:"-"`
	InstallationID   int64      `json:"installation_id"`
	AccessTokenEnc   string     `json:"-"`
	StateHash        string     `json:"-"`
	StateExpiresAt   *time.Time `json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
}

const sourceCols = `id::text, team_id::text, type, name, html_url, api_url, organization, is_public, app_id, app_slug, client_id,
	client_secret_enc, webhook_secret_enc, private_key_enc, installation_id, access_token_enc, state_hash, state_expires_at, created_at`

func scanSource(row pgx.Row) (*GitSource, error) {
	g := &GitSource{}
	err := row.Scan(&g.ID, &g.TeamID, &g.Type, &g.Name, &g.HTMLURL, &g.APIURL, &g.Organization, &g.IsPublic, &g.AppID, &g.AppSlug,
		&g.ClientID, &g.ClientSecretEnc, &g.WebhookSecretEnc, &g.PrivateKeyEnc, &g.InstallationID, &g.AccessTokenEnc,
		&g.StateHash, &g.StateExpiresAt, &g.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return g, err
}

func (s *Store) CreateGitSource(ctx context.Context, g *GitSource) (*GitSource, error) {
	r, err := scanSource(s.Pool.QueryRow(ctx, `INSERT INTO git_sources (team_id, type, name, html_url, api_url, organization, is_public,
		app_id, app_slug, client_id, client_secret_enc, webhook_secret_enc, private_key_enc, installation_id, access_token_enc,
		state_hash, state_expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING `+sourceCols,
		g.TeamID, g.Type, g.Name, g.HTMLURL, g.APIURL, g.Organization, g.IsPublic, g.AppID, g.AppSlug, g.ClientID, g.ClientSecretEnc,
		g.WebhookSecretEnc, g.PrivateKeyEnc, g.InstallationID, g.AccessTokenEnc, g.StateHash, g.StateExpiresAt))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return r, err
}

func (s *Store) UpdateGitSource(ctx context.Context, g *GitSource) error {
	_, err := s.Pool.Exec(ctx, `UPDATE git_sources SET name = $2, html_url = $3, api_url = $4, organization = $5, is_public = $6,
		app_id = $7, app_slug = $8, client_id = $9, client_secret_enc = $10, webhook_secret_enc = $11, private_key_enc = $12,
		installation_id = $13, access_token_enc = $14, state_hash = $15, state_expires_at = $16, updated_at = now() WHERE id = $1`,
		g.ID, g.Name, g.HTMLURL, g.APIURL, g.Organization, g.IsPublic, g.AppID, g.AppSlug, g.ClientID, g.ClientSecretEnc,
		g.WebhookSecretEnc, g.PrivateKeyEnc, g.InstallationID, g.AccessTokenEnc, g.StateHash, g.StateExpiresAt)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) ListGitSources(ctx context.Context, teamID string) ([]*GitSource, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+sourceCols+` FROM git_sources WHERE team_id = $1 ORDER BY lower(name)`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSource)
}

func (s *Store) GitSourceByID(ctx context.Context, teamID, id string) (*GitSource, error) {
	return scanSource(s.Pool.QueryRow(ctx, `SELECT `+sourceCols+` FROM git_sources WHERE team_id = $1 AND id = $2`, teamID, id))
}

// GitSourceByState retrouve la source d'un flux de création GitHub App en cours.
func (s *Store) GitSourceByState(ctx context.Context, stateHash string) (*GitSource, error) {
	return scanSource(s.Pool.QueryRow(ctx, `SELECT `+sourceCols+` FROM git_sources WHERE state_hash = $1 AND state_hash <> ''
		AND state_expires_at > now()`, stateHash))
}

// GitSourcesByAppID : sources GitHub correspondant à un identifiant d'App (webhooks).
func (s *Store) GitSourcesByAppID(ctx context.Context, appID int64) ([]*GitSource, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+sourceCols+` FROM git_sources WHERE type = 'github' AND app_id = $1`, appID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSource)
}

func (s *Store) DeleteGitSource(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM git_sources WHERE team_id = $1 AND id = $2`, teamID, id)
	if isForeignKeyViolation(err) {
		return ErrInUse
	}
	return err
}
