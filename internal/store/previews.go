package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Preview struct {
	ID            string   `json:"id"`
	ApplicationID string   `json:"application_id"`
	PRNumber      int      `json:"pr_number"`
	PRURL         string   `json:"pr_url"`
	Title         string   `json:"title"`
	Domains       []string `json:"domains"`
	// ComposeDomains : domaines par service (applications Docker Compose).
	ComposeDomains map[string][]string `json:"compose_domains"`
	Status         string              `json:"status"`
	LastCommit     string              `json:"last_commit"`
	CommentID      int64               `json:"-"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
}

const previewCols = `id::text, application_id::text, pr_number, pr_url, title, domains, compose_domains, status, last_commit, comment_id,
	created_at, updated_at`

func scanPreview(row pgx.Row) (*Preview, error) {
	p := &Preview{}
	err := row.Scan(&p.ID, &p.ApplicationID, &p.PRNumber, &p.PRURL, &p.Title, &p.Domains, &p.ComposeDomains, &p.Status, &p.LastCommit,
		&p.CommentID, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

func (s *Store) PreviewByNumber(ctx context.Context, appID string, pr int) (*Preview, error) {
	return scanPreview(s.Pool.QueryRow(ctx, `SELECT `+previewCols+` FROM application_previews WHERE application_id = $1 AND pr_number = $2`, appID, pr))
}

func (s *Store) ListPreviews(ctx context.Context, appID string) ([]*Preview, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+previewCols+` FROM application_previews WHERE application_id = $1 ORDER BY pr_number DESC`, appID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanPreview)
}

// UpsertPreview crée ou met à jour la prévisualisation d'une PR (les domaines ne sont fixés qu'à la création).
func (s *Store) UpsertPreview(ctx context.Context, p *Preview) (*Preview, error) {
	if p.ComposeDomains == nil {
		p.ComposeDomains = map[string][]string{}
	}
	return scanPreview(s.Pool.QueryRow(ctx, `INSERT INTO application_previews (application_id, pr_number, pr_url, title, domains,
		compose_domains, last_commit) VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (application_id, pr_number) DO UPDATE SET
		pr_url = EXCLUDED.pr_url, title = EXCLUDED.title, last_commit = EXCLUDED.last_commit, updated_at = now() RETURNING `+previewCols,
		p.ApplicationID, p.PRNumber, p.PRURL, p.Title, p.Domains, p.ComposeDomains, p.LastCommit))
}

func (s *Store) UpdatePreviewDomains(ctx context.Context, appID string, pr int, domains []string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE application_previews SET domains = $3, updated_at = now() WHERE application_id = $1 AND pr_number = $2`,
		appID, pr, domains)
	return err
}

func (s *Store) SetPreviewStatus(ctx context.Context, appID string, pr int, status string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE application_previews SET status = $3, updated_at = now() WHERE application_id = $1 AND pr_number = $2`,
		appID, pr, status)
	return err
}

func (s *Store) DeletePreview(ctx context.Context, appID string, pr int) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM application_previews WHERE application_id = $1 AND pr_number = $2`, appID, pr)
	return err
}

func (s *Store) SetPreviewComment(ctx context.Context, appID string, pr int, commentID int64) error {
	_, err := s.Pool.Exec(ctx, `UPDATE application_previews SET comment_id = $3 WHERE application_id = $1 AND pr_number = $2`,
		appID, pr, commentID)
	return err
}

func (s *Store) UpdatePreviewComposeDomains(ctx context.Context, appID string, pr int, domains map[string][]string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE application_previews SET compose_domains = $3, updated_at = now()
		WHERE application_id = $1 AND pr_number = $2`, appID, pr, domains)
	return err
}
