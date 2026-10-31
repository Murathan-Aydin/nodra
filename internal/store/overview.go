package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// ResourceSummary : ligne de la liste unifiée des ressources d'une équipe.
type ResourceSummary struct {
	ID              string    `json:"id"`
	Type            string    `json:"type"` // application | database | service
	Name            string    `json:"name"`
	Status          string    `json:"status"`
	EnvironmentID   string    `json:"environment_id"`
	EnvironmentName string    `json:"environment_name"`
	ProjectID       string    `json:"project_id"`
	ProjectName     string    `json:"project_name"`
	ServerID        string    `json:"server_id"`
	Domains         []string  `json:"domains"`
	Tags            []string  `json:"tags"`
	CreatedAt       time.Time `json:"created_at"`
}

// ResourceFilter : filtres facultatifs de TeamResources (vides : ignorés).
type ResourceFilter struct {
	Type, EnvironmentID, ServerID, Tag string
}

// unionResources : applications, bases et services avec leurs domaines (services : domaines des sous-services).
const unionResources = `(
	SELECT id, team_id, environment_id, destination_id, name, status, created_at, 'application' AS type, domains FROM applications
	UNION ALL SELECT id, team_id, environment_id, destination_id, name, status, created_at, 'database', '{}'::text[] FROM databases
	UNION ALL SELECT s.id, s.team_id, s.environment_id, s.destination_id, s.name, s.status, s.created_at, 'service',
		coalesce((SELECT array_agg(d ORDER BY d) FROM service_apps sa, unnest(sa.domains) d WHERE sa.service_id = s.id), '{}')
		FROM services s
) x`

// TeamResources liste les ressources de l'équipe (projet, environnement, serveur, domaines, tags).
func (s *Store) TeamResources(ctx context.Context, teamID string, f ResourceFilter) ([]*ResourceSummary, error) {
	rows, err := s.Pool.Query(ctx, `SELECT x.id::text, x.type, x.name, x.status, x.environment_id::text, e.name, p.id::text, p.name,
		d.server_id::text, x.domains, coalesce((SELECT array_agg(t.name ORDER BY lower(t.name)) FROM resource_tags rt
			JOIN tags t ON t.id = rt.tag_id WHERE rt.resource_id = x.id), '{}'), x.created_at
		FROM `+unionResources+`
		JOIN environments e ON e.id = x.environment_id JOIN projects p ON p.id = e.project_id
		JOIN destinations d ON d.id = x.destination_id
		WHERE x.team_id = $1 AND ($2 = '' OR x.type = $2) AND ($3 = '' OR x.environment_id::text = $3)
		  AND ($4 = '' OR d.server_id::text = $4)
		  AND ($5 = '' OR EXISTS (SELECT 1 FROM resource_tags rt JOIN tags t ON t.id = rt.tag_id
		                          WHERE rt.resource_id = x.id AND lower(t.name) = lower($5)))
		ORDER BY lower(p.name), lower(e.name), lower(x.name)`, teamID, f.Type, f.EnvironmentID, f.ServerID, f.Tag)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ResourceSummary{}
	for rows.Next() {
		r := &ResourceSummary{}
		if err := rows.Scan(&r.ID, &r.Type, &r.Name, &r.Status, &r.EnvironmentID, &r.EnvironmentName, &r.ProjectID, &r.ProjectName,
			&r.ServerID, &r.Domains, &r.Tags, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ResourcesByTags : identifiants des ressources de l'équipe portant l'un des tags (noms insensibles à la casse).
func (s *Store) ResourcesByTags(ctx context.Context, teamID string, tags []string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT rt.resource_id::text FROM resource_tags rt JOIN tags t ON t.id = rt.tag_id
		WHERE t.team_id = $1 AND lower(t.name) = ANY(SELECT lower(n) FROM unnest($2::text[]) n) ORDER BY 1`, teamID, tags)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// TeamResourceType : type d'une ressource de l'équipe (ErrNotFound sinon).
func (s *Store) TeamResourceType(ctx context.Context, teamID, id string) (string, error) {
	var typ string
	err := s.Pool.QueryRow(ctx, `SELECT type FROM resources WHERE team_id = $1 AND id::text = $2`, teamID, id).Scan(&typ)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return typ, err
}

// TeamDeployments : déploiements récents de l'équipe (statut facultatif : queued, in_progress, finished…).
func (s *Store) TeamDeployments(ctx context.Context, teamID, status string, limit int) ([]*Deployment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+depCols+` FROM deployments WHERE team_id = $1 AND ($2 = '' OR status = $2)
		ORDER BY created_at DESC LIMIT $3`, teamID, status, limit)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanDeployment)
}

// MoveResource déplace une ressource vers un autre environnement de la même équipe
// (table : constante applications, databases ou services).
func (s *Store) MoveResource(ctx context.Context, table, teamID, id, environmentID string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE `+table+` SET environment_id = $3 WHERE id = $2 AND team_id = $1
		AND EXISTS (SELECT 1 FROM environments e JOIN projects p ON p.id = e.project_id WHERE e.id = $3 AND p.team_id = $1)`,
		teamID, id, environmentID)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
