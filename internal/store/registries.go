package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Registry : registre Docker privé d'une équipe (hérité par ses sous-équipes).
type Registry struct {
	ID          string    `json:"id"`
	TeamID      string    `json:"team_id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Username    string    `json:"username"`
	PasswordEnc string    `json:"-"`
	CreatedAt   time.Time `json:"created_at"`
}

const registryCols = `id::text, team_id::text, name, host, username, password_enc, created_at`

func scanRegistry(row pgx.Row) (*Registry, error) {
	g := &Registry{}
	err := row.Scan(&g.ID, &g.TeamID, &g.Name, &g.Host, &g.Username, &g.PasswordEnc, &g.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return g, err
}

func (s *Store) CreateRegistry(ctx context.Context, g *Registry) (*Registry, error) {
	r, err := scanRegistry(s.Pool.QueryRow(ctx, `INSERT INTO docker_registries (team_id, name, host, username, password_enc)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+registryCols, g.TeamID, g.Name, g.Host, g.Username, g.PasswordEnc))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return r, err
}

func (s *Store) UpdateRegistry(ctx context.Context, g *Registry) error {
	_, err := s.Pool.Exec(ctx, `UPDATE docker_registries SET name = $3, host = $4, username = $5, password_enc = $6
		WHERE team_id = $1 AND id = $2`, g.TeamID, g.ID, g.Name, g.Host, g.Username, g.PasswordEnc)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) RegistryByID(ctx context.Context, teamID, id string) (*Registry, error) {
	return scanRegistry(s.Pool.QueryRow(ctx, `SELECT `+registryCols+` FROM docker_registries WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) ListRegistries(ctx context.Context, teamID string) ([]*Registry, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+registryCols+` FROM docker_registries WHERE team_id = $1 ORDER BY lower(name)`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanRegistry)
}

func (s *Store) DeleteRegistry(ctx context.Context, teamID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM docker_registries WHERE team_id = $1 AND id = $2`, teamID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// RegistriesFor : registres utilisables par une équipe (les siens, puis ceux de ses équipes parentes ;
// pour un même hôte, le plus proche l'emporte).
func (s *Store) RegistriesFor(ctx context.Context, teamID string) ([]*Registry, error) {
	chain, err := s.TeamChain(ctx, teamID)
	if err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+registryCols+` FROM docker_registries WHERE team_id = ANY($1::uuid[])
		ORDER BY array_position($1::uuid[], team_id), host`, chain)
	if err != nil {
		return nil, err
	}
	all, err := collect(rows, scanRegistry)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	out := []*Registry{}
	for _, g := range all {
		if !seen[g.Host] {
			seen[g.Host] = true
			out = append(out, g)
		}
	}
	return out, nil
}
