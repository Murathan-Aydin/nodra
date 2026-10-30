package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
)

type Team struct {
	ID          string    `json:"id"`
	ParentID    *string   `json:"parent_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Personal    bool      `json:"personal"`
	CreatedAt   time.Time `json:"created_at"`
}

const teamCols = `id::text, parent_id::text, name, description, personal, created_at`

func scanTeam(row pgx.Row) (*Team, error) {
	t := &Team{}
	err := row.Scan(&t.ID, &t.ParentID, &t.Name, &t.Description, &t.Personal, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// createTeamTx crée une équipe dont ownerID devient membre et propriétaire.
func createTeamTx(ctx context.Context, tx pgx.Tx, name, desc string, parentID *string, personal bool, ownerID string) (*Team, error) {
	t, err := scanTeam(tx.QueryRow(ctx, `INSERT INTO teams (parent_id, name, description, personal)
		VALUES ($1, $2, $3, $4) RETURNING `+teamCols, parentID, name, desc, personal))
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO team_members (team_id, user_id) VALUES ($1, $2)`, t.ID, ownerID); err != nil {
		return nil, err
	}
	tag, err := tx.Exec(ctx, `INSERT INTO role_assignments (team_id, user_id, role_id, scope_type, scope_id, created_by)
		SELECT $1, $2, id, 'team', $1, $2 FROM roles WHERE key = $3 AND team_id IS NULL`, t.ID, ownerID, rbac.RoleOwner)
	if err == nil && tag.RowsAffected() != 1 {
		err = errors.New("rôle système owner absent (SyncSystemRoles non exécuté ?)")
	}
	return t, err
}

func (s *Store) CreateTeam(ctx context.Context, name, desc string, parentID *string, ownerID string) (*Team, error) {
	var t *Team
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		t, err = createTeamTx(ctx, tx, name, desc, parentID, false, ownerID)
		return err
	})
	return t, err
}

func (s *Store) TeamByID(ctx context.Context, id string) (*Team, error) {
	return scanTeam(s.Pool.QueryRow(ctx, `SELECT `+teamCols+` FROM teams WHERE id = $1`, id))
}

// TeamChain renvoie l'équipe puis ses ancêtres jusqu'à l'organisation racine.
func (s *Store) TeamChain(ctx context.Context, id string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `WITH RECURSIVE c AS (
			SELECT id, parent_id, 0 AS depth FROM teams WHERE id = $1
			UNION ALL
			SELECT t.id, t.parent_id, c.depth + 1 FROM teams t JOIN c ON t.id = c.parent_id
		) SELECT id::text FROM c ORDER BY depth`, id)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err == nil && len(ids) == 0 {
		return nil, ErrNotFound
	}
	return ids, err
}

// TeamSubtree renvoie l'équipe et toutes ses descendantes.
func (s *Store) TeamSubtree(ctx context.Context, id string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `WITH RECURSIVE c AS (
			SELECT id FROM teams WHERE id = $1
			UNION ALL
			SELECT t.id FROM teams t JOIN c ON t.parent_id = c.id
		) SELECT id::text FROM c`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// UserTeams : équipes dont l'utilisateur est membre, directement ou via une équipe parente.
func (s *Store) UserTeams(ctx context.Context, userID string) ([]*Team, error) {
	rows, err := s.Pool.Query(ctx, `WITH RECURSIVE t AS (
			SELECT team_id AS id FROM team_members WHERE user_id = $1
			UNION
			SELECT c.id FROM teams c JOIN t ON c.parent_id = t.id
		) SELECT `+teamCols+` FROM teams WHERE id IN (SELECT id FROM t) ORDER BY personal DESC, name`, userID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanTeam)
}

func (s *Store) IsMember(ctx context.Context, userID string, chain []string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM team_members WHERE user_id = $1 AND team_id = ANY($2::uuid[]))`,
		userID, chain).Scan(&ok)
	return ok, err
}

func (s *Store) UpdateTeam(ctx context.Context, id, name, desc string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE teams SET name = $2, description = $3, updated_at = now() WHERE id = $1`, id, name, desc)
	return err
}

func (s *Store) DeleteTeam(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM teams WHERE id = $1`, id)
	return err
}

type MemberRole struct {
	AssignmentID string `json:"assignment_id"`
	RoleID       string `json:"role_id"`
	RoleName     string `json:"role_name"`
	Effect       string `json:"effect"`
	TeamID       string `json:"team_id"` // équipe sur laquelle porte l'attribution (héritage possible)
}

type Member struct {
	UserID    string       `json:"user_id"`
	Email     string       `json:"email"`
	Name      string       `json:"name"`
	TeamID    string       `json:"team_id"` // équipe où l'adhésion est enregistrée
	Inherited bool         `json:"inherited"`
	JoinedAt  time.Time    `json:"joined_at"`
	Roles     []MemberRole `json:"roles"`
}

// Members liste les membres d'une équipe (adhésions directes et héritées des équipes parentes)
// avec leurs rôles attribués directement au niveau équipe.
func (s *Store) Members(ctx context.Context, chain []string) ([]*Member, error) {
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT ON (u.id) u.id::text, u.email, u.name, tm.team_id::text, tm.created_at,
			coalesce((SELECT json_agg(json_build_object('assignment_id', ra.id, 'role_id', r.id, 'role_name', r.name,
				'effect', ra.effect, 'team_id', ra.scope_id) ORDER BY r.name)
			  FROM role_assignments ra JOIN roles r ON r.id = ra.role_id
			  WHERE ra.user_id = u.id AND ra.scope_type = 'team' AND ra.scope_id = ANY($1::uuid[])), '[]')
		FROM team_members tm JOIN users u ON u.id = tm.user_id
		WHERE tm.team_id = ANY($1::uuid[])
		ORDER BY u.id, array_position($1::uuid[], tm.team_id)`, chain)
	if err != nil {
		return nil, err
	}
	return collect(rows, func(row pgx.Row) (*Member, error) {
		m := &Member{}
		err := row.Scan(&m.UserID, &m.Email, &m.Name, &m.TeamID, &m.JoinedAt, &m.Roles)
		m.Inherited = m.TeamID != chain[0]
		return m, err
	})
}

func (s *Store) IsDirectMember(ctx context.Context, teamID, userID string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $2)`, teamID, userID).Scan(&ok)
	return ok, err
}

func (s *Store) AddMember(ctx context.Context, teamID, userID string) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO team_members (team_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, teamID, userID)
	return err
}

// RemoveMember retire l'utilisateur de l'équipe et de ses sous-équipes (adhésions, rôles, groupes).
func (s *Store) RemoveMember(ctx context.Context, teamID, userID string) error {
	subtree, err := s.TeamSubtree(ctx, teamID)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		for _, q := range []string{
			`DELETE FROM team_members WHERE user_id = $1 AND team_id = ANY($2::uuid[])`,
			`DELETE FROM role_assignments WHERE user_id = $1 AND team_id = ANY($2::uuid[])`,
			`DELETE FROM group_members WHERE user_id = $1 AND group_id IN (SELECT id FROM groups WHERE team_id = ANY($2::uuid[]))`,
		} {
			if _, err := tx.Exec(ctx, q, userID, subtree); err != nil {
				return err
			}
		}
		return nil
	})
}

// OwnerIDs renvoie les propriétaires directs (rôle owner attribué sur l'équipe elle-même).
func (s *Store) OwnerIDs(ctx context.Context, teamID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT ra.user_id::text FROM role_assignments ra JOIN roles r ON r.id = ra.role_id
		WHERE r.team_id IS NULL AND r.key = $2 AND ra.scope_type = 'team' AND ra.scope_id = $1
		  AND ra.effect = 'allow' AND ra.user_id IS NOT NULL`, teamID, rbac.RoleOwner)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// TransferOwnership donne le rôle owner à toID et rétrograde fromID en admin.
func (s *Store) TransferOwnership(ctx context.Context, teamID, fromID, toID string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO role_assignments (team_id, user_id, role_id, scope_type, scope_id, created_by)
			SELECT $1, $2, id, 'team', $1, $3 FROM roles WHERE key = 'owner' AND team_id IS NULL
			ON CONFLICT DO NOTHING`, teamID, toID, fromID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM role_assignments WHERE team_id = $1 AND user_id = $2 AND scope_type = 'team'
			AND scope_id = $1 AND role_id = (SELECT id FROM roles WHERE key = 'owner' AND team_id IS NULL)`, teamID, fromID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO role_assignments (team_id, user_id, role_id, scope_type, scope_id, created_by)
			SELECT $1, $2, id, 'team', $1, $2 FROM roles WHERE key = 'admin' AND team_id IS NULL
			ON CONFLICT DO NOTHING`, teamID, fromID)
		return err
	})
}
