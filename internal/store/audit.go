package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type AuditEvent struct {
	ID           int64           `json:"id"`
	TeamID       *string         `json:"team_id"`
	ActorUserID  *string         `json:"actor_user_id"`
	ActorEmail   string          `json:"actor_email"`
	ActorTokenID *string         `json:"actor_token_id"`
	Action       string          `json:"action"`
	TargetType   string          `json:"target_type"`
	TargetID     string          `json:"target_id"`
	IP           string          `json:"ip"`
	UserAgent    string          `json:"user_agent"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    time.Time       `json:"created_at"`
}

func (s *Store) InsertAudit(ctx context.Context, e *AuditEvent) error {
	meta := e.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO audit_events (team_id, actor_user_id, actor_email, actor_token_id, action,
		target_type, target_id, ip, user_agent, metadata) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)`,
		e.TeamID, e.ActorUserID, e.ActorEmail, e.ActorTokenID, e.Action, e.TargetType, e.TargetID, e.IP, e.UserAgent, string(meta))
	return err
}

type AuditFilter struct {
	TeamIDs     []string // équipe et sous-équipes
	ActorUserID string
	Action      string // préfixe, ex. "team.member"
	TargetType  string
	TargetID    string
	Since       *time.Time
	Until       *time.Time
	Before      int64 // curseur : id strictement inférieur
	Limit       int
}

func (s *Store) ListAudit(ctx context.Context, f AuditFilter) ([]*AuditEvent, error) {
	var where []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	add("team_id = ANY($%d::uuid[])", f.TeamIDs)
	if f.ActorUserID != "" {
		add("actor_user_id::text = $%d", f.ActorUserID)
	}
	if f.Action != "" {
		add("action LIKE $%d || '%%'", f.Action)
	}
	if f.TargetType != "" {
		add("target_type = $%d", f.TargetType)
	}
	if f.TargetID != "" {
		add("target_id = $%d", f.TargetID)
	}
	if f.Since != nil {
		add("created_at >= $%d", *f.Since)
	}
	if f.Until != nil {
		add("created_at < $%d", *f.Until)
	}
	if f.Before > 0 {
		add("id < $%d", f.Before)
	}
	if f.Limit <= 0 {
		f.Limit = 50
	}
	args = append(args, f.Limit)
	rows, err := s.Pool.Query(ctx, fmt.Sprintf(`SELECT id, team_id::text, actor_user_id::text, actor_email, actor_token_id::text,
		action, target_type, target_id, ip, user_agent, metadata, created_at FROM audit_events
		WHERE %s ORDER BY id DESC LIMIT $%d`, strings.Join(where, " AND "), len(args)), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*AuditEvent{}
	for rows.Next() {
		e := &AuditEvent{}
		if err := rows.Scan(&e.ID, &e.TeamID, &e.ActorUserID, &e.ActorEmail, &e.ActorTokenID, &e.Action, &e.TargetType,
			&e.TargetID, &e.IP, &e.UserAgent, &e.Metadata, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
