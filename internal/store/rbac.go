package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
)

// SyncSystemRoles aligne les rôles système en base sur ceux définis dans le code.
func (s *Store) SyncSystemRoles(ctx context.Context) error {
	for _, r := range rbac.SystemRoles() {
		if _, err := s.Pool.Exec(ctx, `INSERT INTO roles (key, name, description, permissions) VALUES ($1, $2, $3, $4)
			ON CONFLICT (key) WHERE team_id IS NULL DO UPDATE
			SET name = EXCLUDED.name, description = EXCLUDED.description, permissions = EXCLUDED.permissions, updated_at = now()`,
			r.Key, r.Name, r.Description, r.Permissions); err != nil {
			return err
		}
	}
	return nil
}

type Role struct {
	ID          string    `json:"id"`
	TeamID      *string   `json:"team_id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	System      bool      `json:"system"`
	CreatedAt   time.Time `json:"created_at"`
}

const roleCols = `id::text, team_id::text, key, name, description, permissions, team_id IS NULL, created_at`

func scanRole(row pgx.Row) (*Role, error) {
	r := &Role{}
	err := row.Scan(&r.ID, &r.TeamID, &r.Key, &r.Name, &r.Description, &r.Permissions, &r.System, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

// UsableIn indique si le rôle peut être attribué dans une équipe (système, ou défini par elle ou un ancêtre).
func (r *Role) UsableIn(chain []string) bool {
	if r.TeamID == nil {
		return true
	}
	for _, id := range chain {
		if id == *r.TeamID {
			return true
		}
	}
	return false
}

func (s *Store) RolesForTeam(ctx context.Context, chain []string) ([]*Role, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+roleCols+` FROM roles WHERE team_id IS NULL OR team_id = ANY($1::uuid[])
		ORDER BY team_id NULLS FIRST, name`, chain)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanRole)
}

func (s *Store) RoleByID(ctx context.Context, id string) (*Role, error) {
	return scanRole(s.Pool.QueryRow(ctx, `SELECT `+roleCols+` FROM roles WHERE id = $1`, id))
}

func (s *Store) SystemRole(ctx context.Context, key string) (*Role, error) {
	return scanRole(s.Pool.QueryRow(ctx, `SELECT `+roleCols+` FROM roles WHERE team_id IS NULL AND key = $1`, key))
}

func (s *Store) CreateRole(ctx context.Context, teamID, name, desc string, perms []string) (*Role, error) {
	r, err := scanRole(s.Pool.QueryRow(ctx, `INSERT INTO roles (team_id, name, description, permissions)
		VALUES ($1, $2, $3, $4) RETURNING `+roleCols, teamID, name, desc, perms))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return r, err
}

func (s *Store) UpdateRole(ctx context.Context, id, name, desc string, perms []string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE roles SET name = $2, description = $3, permissions = $4, updated_at = now()
		WHERE id = $1 AND team_id IS NOT NULL`, id, name, desc, perms)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) DeleteRole(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM roles WHERE id = $1 AND team_id IS NOT NULL`, id)
	return err
}

// --- Attributions ---

type RoleAssignment struct {
	ID          string     `json:"id"`
	TeamID      string     `json:"team_id"`
	UserID      *string    `json:"user_id"`
	GroupID     *string    `json:"group_id"`
	SubjectName string     `json:"subject_name"`
	RoleID      string     `json:"role_id"`
	RoleName    string     `json:"role_name"`
	Scope       rbac.Scope `json:"scope"`
	Effect      string     `json:"effect"`
	CreatedBy   *string    `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
}

const assignmentSelect = `SELECT ra.id::text, ra.team_id::text, ra.user_id::text, ra.group_id::text,
	coalesce(u.email, g.name, ''), ra.role_id::text, r.name, ra.scope_type, ra.scope_id::text, ra.effect,
	ra.created_by::text, ra.created_at
	FROM role_assignments ra JOIN roles r ON r.id = ra.role_id
	LEFT JOIN users u ON u.id = ra.user_id LEFT JOIN groups g ON g.id = ra.group_id`

func scanAssignment(row pgx.Row) (*RoleAssignment, error) {
	a := &RoleAssignment{}
	err := row.Scan(&a.ID, &a.TeamID, &a.UserID, &a.GroupID, &a.SubjectName, &a.RoleID, &a.RoleName,
		&a.Scope.Type, &a.Scope.ID, &a.Effect, &a.CreatedBy, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) ListAssignments(ctx context.Context, teamID string) ([]*RoleAssignment, error) {
	rows, err := s.Pool.Query(ctx, assignmentSelect+` WHERE ra.team_id = $1 ORDER BY ra.scope_type, ra.created_at`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanAssignment)
}

func (s *Store) AssignmentByID(ctx context.Context, teamID, id string) (*RoleAssignment, error) {
	return scanAssignment(s.Pool.QueryRow(ctx, assignmentSelect+` WHERE ra.team_id = $1 AND ra.id = $2`, teamID, id))
}

func (s *Store) CreateAssignment(ctx context.Context, a *RoleAssignment) (*RoleAssignment, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO role_assignments (team_id, user_id, group_id, role_id, scope_type, scope_id, effect, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id::text`,
		a.TeamID, a.UserID, a.GroupID, a.RoleID, a.Scope.Type, a.Scope.ID, a.Effect, a.CreatedBy).Scan(&id)
	if isUniqueViolation(err) {
		return nil, ErrConflict
	} else if err != nil {
		return nil, err
	}
	return s.AssignmentByID(ctx, a.TeamID, id)
}

func (s *Store) DeleteAssignment(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM role_assignments WHERE team_id = $1 AND id = $2`, teamID, id)
	return err
}

// EffectiveAssignments renvoie les attributions de l'utilisateur (directes ou via ses groupes)
// portant sur l'une des portées de la chaîne.
func (s *Store) EffectiveAssignments(ctx context.Context, userID string, chain []rbac.Scope) ([]rbac.Assignment, error) {
	types := make([]string, len(chain))
	ids := make([]string, len(chain))
	for i, c := range chain {
		types[i], ids[i] = c.Type, c.ID
	}
	rows, err := s.Pool.Query(ctx, `SELECT ra.scope_type, ra.scope_id::text, ra.effect,
			CASE WHEN r.team_id IS NULL THEN r.key ELSE '' END, r.permissions
		FROM role_assignments ra
		JOIN roles r ON r.id = ra.role_id
		JOIN unnest($2::text[], $3::text[]) AS c(t, i) ON ra.scope_type = c.t AND ra.scope_id::text = c.i
		WHERE ra.user_id = $1 OR ra.group_id IN (SELECT group_id FROM group_members WHERE user_id = $1)`,
		userID, types, ids)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (rbac.Assignment, error) {
		var a rbac.Assignment
		err := row.Scan(&a.Scope.Type, &a.Scope.ID, &a.Effect, &a.RoleKey, &a.Permissions)
		return a, err
	})
}

// --- Groupes ---

type GroupMember struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Name   string `json:"name"`
}

type Group struct {
	ID          string        `json:"id"`
	TeamID      string        `json:"team_id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Members     []GroupMember `json:"members"`
	CreatedAt   time.Time     `json:"created_at"`
}

const groupSelect = `SELECT g.id::text, g.team_id::text, g.name, g.description, g.created_at,
	coalesce((SELECT json_agg(json_build_object('user_id', u.id, 'email', u.email, 'name', u.name) ORDER BY u.email)
	  FROM group_members gm JOIN users u ON u.id = gm.user_id WHERE gm.group_id = g.id), '[]')
	FROM groups g`

func scanGroup(row pgx.Row) (*Group, error) {
	g := &Group{}
	err := row.Scan(&g.ID, &g.TeamID, &g.Name, &g.Description, &g.CreatedAt, &g.Members)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return g, err
}

func (s *Store) ListGroups(ctx context.Context, teamID string) ([]*Group, error) {
	rows, err := s.Pool.Query(ctx, groupSelect+` WHERE g.team_id = $1 ORDER BY g.name`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanGroup)
}

// GroupInChain renvoie un groupe défini par l'équipe ou l'un de ses ancêtres.
func (s *Store) GroupInChain(ctx context.Context, chain []string, id string) (*Group, error) {
	return scanGroup(s.Pool.QueryRow(ctx, groupSelect+` WHERE g.team_id = ANY($1::uuid[]) AND g.id = $2`, chain, id))
}

func (s *Store) CreateGroup(ctx context.Context, teamID, name, desc string) (*Group, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO groups (team_id, name, description) VALUES ($1, $2, $3) RETURNING id::text`,
		teamID, name, desc).Scan(&id)
	if isUniqueViolation(err) {
		return nil, ErrConflict
	} else if err != nil {
		return nil, err
	}
	return s.GroupInChain(ctx, []string{teamID}, id)
}

func (s *Store) UpdateGroup(ctx context.Context, id, name, desc string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE groups SET name = $2, description = $3, updated_at = now() WHERE id = $1`, id, name, desc)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) DeleteGroup(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM groups WHERE id = $1`, id)
	return err
}

func (s *Store) AddGroupMember(ctx context.Context, groupID, userID string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, groupID, userID)
	return err
}

func (s *Store) RemoveGroupMember(ctx context.Context, groupID, userID string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	return err
}
