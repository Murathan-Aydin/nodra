package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Invitation struct {
	ID        string    `json:"id"`
	TeamID    string    `json:"team_id"`
	TeamName  string    `json:"team_name"`
	Email     string    `json:"email"`
	RoleID    string    `json:"role_id"`
	RoleName  string    `json:"role_name"`
	GroupIDs  []string  `json:"group_ids"`
	TokenHash string    `json:"-"`
	InvitedBy *string   `json:"invited_by"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

const invitationSelect = `SELECT i.id::text, i.team_id::text, t.name, i.email, i.role_id::text, r.name, i.group_ids::text[],
	i.token_hash, i.invited_by::text, i.expires_at, i.created_at
	FROM team_invitations i JOIN teams t ON t.id = i.team_id JOIN roles r ON r.id = i.role_id`

func scanInvitation(row pgx.Row) (*Invitation, error) {
	i := &Invitation{}
	err := row.Scan(&i.ID, &i.TeamID, &i.TeamName, &i.Email, &i.RoleID, &i.RoleName, &i.GroupIDs, &i.TokenHash,
		&i.InvitedBy, &i.ExpiresAt, &i.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return i, err
}

// CreateInvitation crée l'invitation (ou remplace celle déjà envoyée à la même adresse).
func (s *Store) CreateInvitation(ctx context.Context, i *Invitation) (*Invitation, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO team_invitations (team_id, email, role_id, group_ids, token_hash, invited_by, expires_at)
		VALUES ($1, $2, $3, $4::uuid[], $5, $6, $7)
		ON CONFLICT (team_id, lower(email)) DO UPDATE SET role_id = EXCLUDED.role_id, group_ids = EXCLUDED.group_ids,
			token_hash = EXCLUDED.token_hash, invited_by = EXCLUDED.invited_by, expires_at = EXCLUDED.expires_at, created_at = now()
		RETURNING id::text`, i.TeamID, i.Email, i.RoleID, i.GroupIDs, i.TokenHash, i.InvitedBy, i.ExpiresAt).Scan(&id)
	if err != nil {
		return nil, err
	}
	return scanInvitation(s.Pool.QueryRow(ctx, invitationSelect+` WHERE i.id = $1`, id))
}

func (s *Store) ListInvitations(ctx context.Context, teamID string) ([]*Invitation, error) {
	rows, err := s.Pool.Query(ctx, invitationSelect+` WHERE i.team_id = $1 ORDER BY i.created_at DESC`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanInvitation)
}

func (s *Store) DeleteInvitation(ctx context.Context, teamID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM team_invitations WHERE team_id = $1 AND id = $2`, teamID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) InvitationByToken(ctx context.Context, hash string) (*Invitation, error) {
	return scanInvitation(s.Pool.QueryRow(ctx, invitationSelect+` WHERE i.token_hash = $1 AND i.expires_at > now()`, hash))
}

// AcceptInvitation ajoute l'utilisateur à l'équipe avec le rôle et les groupes prévus, puis supprime l'invitation.
func (s *Store) AcceptInvitation(ctx context.Context, i *Invitation, userID string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `DELETE FROM team_invitations WHERE id = $1`, i.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound // déjà utilisée
		}
		if _, err := tx.Exec(ctx, `INSERT INTO team_members (team_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, i.TeamID, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO role_assignments (team_id, user_id, role_id, scope_type, scope_id, created_by)
			VALUES ($1, $2, $3, 'team', $1, $4) ON CONFLICT DO NOTHING`, i.TeamID, userID, i.RoleID, i.InvitedBy); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO group_members (group_id, user_id)
			SELECT id, $2 FROM groups WHERE id = ANY($1::uuid[]) ON CONFLICT DO NOTHING`, i.GroupIDs, userID)
		return err
	})
}
