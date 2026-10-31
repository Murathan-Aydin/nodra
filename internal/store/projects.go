package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
)

func init() {
	scopeParents[rbac.ScopeProject] = func(ctx context.Context, s *Store, id string) (rbac.Scope, error) {
		var team string
		err := s.Pool.QueryRow(ctx, `SELECT team_id::text FROM projects WHERE id = $1`, id).Scan(&team)
		if errors.Is(err, pgx.ErrNoRows) {
			return rbac.Scope{}, ErrNotFound
		}
		return rbac.Scope{Type: rbac.ScopeTeam, ID: team}, err
	}
	scopeParents[rbac.ScopeEnvironment] = func(ctx context.Context, s *Store, id string) (rbac.Scope, error) {
		var project string
		err := s.Pool.QueryRow(ctx, `SELECT project_id::text FROM environments WHERE id = $1`, id).Scan(&project)
		if errors.Is(err, pgx.ErrNoRows) {
			return rbac.Scope{}, ErrNotFound
		}
		return rbac.Scope{Type: rbac.ScopeProject, ID: project}, err
	}
}

type Project struct {
	ID          string    `json:"id"`
	TeamID      string    `json:"team_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	// EnvironmentCount : nombre d'environnements (tableau de bord).
	EnvironmentCount int `json:"environment_count"`
}

const projectCols = `id::text, team_id::text, name, description, created_at,
	(SELECT count(*) FROM environments e WHERE e.project_id = projects.id)`

func scanProject(row pgx.Row) (*Project, error) {
	p := &Project{}
	err := row.Scan(&p.ID, &p.TeamID, &p.Name, &p.Description, &p.CreatedAt, &p.EnvironmentCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return p, err
}

// CreateProject crée le projet avec un environnement « production ».
func (s *Store) CreateProject(ctx context.Context, teamID, name, desc string) (*Project, error) {
	var p *Project
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		if p, err = scanProject(tx.QueryRow(ctx, `INSERT INTO projects (team_id, name, description) VALUES ($1, $2, $3)
			RETURNING `+projectCols, teamID, name, desc)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO environments (project_id, name) VALUES ($1, 'production')`, p.ID)
		return err
	})
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return p, err
}

func (s *Store) ListProjects(ctx context.Context, teamID string) ([]*Project, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+projectCols+` FROM projects WHERE team_id = $1 ORDER BY lower(name)`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanProject)
}

func (s *Store) ProjectByID(ctx context.Context, teamID, id string) (*Project, error) {
	return scanProject(s.Pool.QueryRow(ctx, `SELECT `+projectCols+` FROM projects WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) UpdateProject(ctx context.Context, teamID, id, name, desc string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE projects SET name = $3, description = $4, updated_at = now() WHERE team_id = $1 AND id = $2`,
		teamID, id, name, desc)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// DeleteProject supprime le projet et ses environnements ; ErrInUse s'il contient encore des ressources.
func (s *Store) DeleteProject(ctx context.Context, teamID, id string) error {
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM shared_variables WHERE (scope_type = 'project' AND scope_id = $1)
			OR (scope_type = 'environment' AND scope_id IN (SELECT id FROM environments WHERE project_id = $1))`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_assignments WHERE (scope_type = 'project' AND scope_id = $1)
			OR (scope_type = 'environment' AND scope_id IN (SELECT id FROM environments WHERE project_id = $1))`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM projects WHERE team_id = $1 AND id = $2`, teamID, id)
		return err
	})
	if isForeignKeyViolation(err) {
		return ErrInUse
	}
	return err
}

type Environment struct {
	ID          string    `json:"id"`
	ProjectID   string    `json:"project_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

const envCols = `id::text, project_id::text, name, description, created_at`

func scanEnv(row pgx.Row) (*Environment, error) {
	e := &Environment{}
	err := row.Scan(&e.ID, &e.ProjectID, &e.Name, &e.Description, &e.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

func (s *Store) ListEnvironments(ctx context.Context, projectID string) ([]*Environment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+envCols+` FROM environments WHERE project_id = $1 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanEnv)
}

func (s *Store) EnvironmentByID(ctx context.Context, projectID, id string) (*Environment, error) {
	return scanEnv(s.Pool.QueryRow(ctx, `SELECT `+envCols+` FROM environments WHERE project_id = $1 AND id = $2`, projectID, id))
}

func (s *Store) CreateEnvironment(ctx context.Context, projectID, name, desc string) (*Environment, error) {
	e, err := scanEnv(s.Pool.QueryRow(ctx, `INSERT INTO environments (project_id, name, description) VALUES ($1, $2, $3)
		RETURNING `+envCols, projectID, name, desc))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return e, err
}

func (s *Store) UpdateEnvironment(ctx context.Context, projectID, id, name, desc string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE environments SET name = $3, description = $4, updated_at = now() WHERE project_id = $1 AND id = $2`,
		projectID, id, name, desc)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// DeleteEnvironment supprime l'environnement ; ErrInUse s'il contient encore des ressources.
func (s *Store) DeleteEnvironment(ctx context.Context, projectID, id string) error {
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM shared_variables WHERE scope_type = 'environment' AND scope_id = $1`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_assignments WHERE scope_type = 'environment' AND scope_id = $1`, id); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM environments WHERE project_id = $1 AND id = $2`, projectID, id)
		return err
	})
	if isForeignKeyViolation(err) {
		return ErrInUse
	}
	return err
}

// CloneEnvironment copie un environnement (et ses variables partagées) dans un projet de la même équipe.
// Les ressources sont clonées par leurs modules respectifs.
func (s *Store) CloneEnvironment(ctx context.Context, src *Environment, targetProjectID, name string) (*Environment, error) {
	var e *Environment
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		if e, err = scanEnv(tx.QueryRow(ctx, `INSERT INTO environments (project_id, name, description) VALUES ($1, $2, $3)
			RETURNING `+envCols, targetProjectID, name, src.Description)); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO shared_variables (team_id, scope_type, scope_id, key, value_enc, comment, is_secret, is_multiline, is_literal)
			SELECT team_id, scope_type, $2, key, value_enc, comment, is_secret, is_multiline, is_literal
			FROM shared_variables WHERE scope_type = 'environment' AND scope_id = $1`, src.ID, e.ID)
		return err
	})
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return e, err
}

// --- Variables partagées ---

type SharedVariable struct {
	ID          string     `json:"id"`
	TeamID      string     `json:"team_id"`
	Scope       rbac.Scope `json:"scope"`
	Key         string     `json:"key"`
	ValueEnc    string     `json:"-"`
	Value       string     `json:"value"` // vide pour un secret
	Comment     string     `json:"comment"`
	IsSecret    bool       `json:"is_secret"`
	IsMultiline bool       `json:"is_multiline"`
	IsLiteral   bool       `json:"is_literal"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

const varCols = `id::text, team_id::text, scope_type, scope_id::text, key, value_enc, comment, is_secret, is_multiline, is_literal, updated_at`

func scanVar(row pgx.Row) (*SharedVariable, error) {
	v := &SharedVariable{}
	err := row.Scan(&v.ID, &v.TeamID, &v.Scope.Type, &v.Scope.ID, &v.Key, &v.ValueEnc, &v.Comment, &v.IsSecret, &v.IsMultiline,
		&v.IsLiteral, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

func (s *Store) ListSharedVariables(ctx context.Context, teamID string, sc rbac.Scope) ([]*SharedVariable, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+varCols+` FROM shared_variables WHERE team_id = $1 AND scope_type = $2 AND scope_id = $3
		ORDER BY key`, teamID, sc.Type, sc.ID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanVar)
}

func (s *Store) SharedVariableByID(ctx context.Context, teamID, id string) (*SharedVariable, error) {
	return scanVar(s.Pool.QueryRow(ctx, `SELECT `+varCols+` FROM shared_variables WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) CreateSharedVariable(ctx context.Context, v *SharedVariable) (*SharedVariable, error) {
	r, err := scanVar(s.Pool.QueryRow(ctx, `INSERT INTO shared_variables (team_id, scope_type, scope_id, key, value_enc, comment,
		is_secret, is_multiline, is_literal) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+varCols,
		v.TeamID, v.Scope.Type, v.Scope.ID, v.Key, v.ValueEnc, v.Comment, v.IsSecret, v.IsMultiline, v.IsLiteral))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return r, err
}

func (s *Store) UpdateSharedVariable(ctx context.Context, v *SharedVariable) error {
	_, err := s.Pool.Exec(ctx, `UPDATE shared_variables SET value_enc = $3, comment = $4, is_secret = $5, is_multiline = $6,
		is_literal = $7, updated_at = now() WHERE team_id = $1 AND id = $2`,
		v.TeamID, v.ID, v.ValueEnc, v.Comment, v.IsSecret, v.IsMultiline, v.IsLiteral)
	return err
}

func (s *Store) DeleteSharedVariable(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM shared_variables WHERE team_id = $1 AND id = $2`, teamID, id)
	return err
}

// SharedVariablesFor renvoie les variables partagées applicables à une ressource, indexées par
// "team.CLÉ", "project.CLÉ", "environment.CLÉ", "server.CLÉ" (valeurs encore chiffrées).
func (s *Store) SharedVariablesFor(ctx context.Context, teamID, projectID, envID, serverID string) (map[string]*SharedVariable, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+varCols+` FROM shared_variables WHERE team_id = $1 AND (
		(scope_type = 'team' AND scope_id = $1) OR (scope_type = 'project' AND scope_id::text = $2)
		OR (scope_type = 'environment' AND scope_id::text = $3) OR (scope_type = 'server' AND scope_id::text = $4))`,
		teamID, projectID, envID, serverID)
	if err != nil {
		return nil, err
	}
	list, err := collect(rows, scanVar)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*SharedVariable, len(list))
	for _, v := range list {
		out[v.Scope.Type+"."+v.Key] = v
	}
	return out, nil
}

// --- Tags ---

type Tag struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"team_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	CreatedAt time.Time `json:"created_at"`
}

func scanTag(row pgx.Row) (*Tag, error) {
	t := &Tag{}
	err := row.Scan(&t.ID, &t.TeamID, &t.Name, &t.Color, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Store) ListTags(ctx context.Context, teamID string) ([]*Tag, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, team_id::text, name, color, created_at FROM tags WHERE team_id = $1 ORDER BY lower(name)`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanTag)
}

func (s *Store) CreateTag(ctx context.Context, teamID, name, color string) (*Tag, error) {
	t, err := scanTag(s.Pool.QueryRow(ctx, `INSERT INTO tags (team_id, name, color) VALUES ($1, $2, $3)
		RETURNING id::text, team_id::text, name, color, created_at`, teamID, name, color))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return t, err
}

func (s *Store) UpdateTag(ctx context.Context, teamID, id, name, color string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE tags SET name = $3, color = $4 WHERE team_id = $1 AND id = $2`, teamID, id, name, color)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteTag(ctx context.Context, teamID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM tags WHERE team_id = $1 AND id = $2`, teamID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
