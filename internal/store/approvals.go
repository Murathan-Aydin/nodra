package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
)

type ApprovalRule struct {
	ID                string     `json:"id"`
	TeamID            string     `json:"team_id"`
	Name              string     `json:"name"`
	Scope             rbac.Scope `json:"scope"`
	Actions           []string   `json:"actions"`
	RequiredApprovals int        `json:"required_approvals"`
	Enabled           bool       `json:"enabled"`
	CreatedAt         time.Time  `json:"created_at"`
}

const ruleCols = `id::text, team_id::text, name, scope_type, scope_id::text, actions, required_approvals, enabled, created_at`

func scanRule(row pgx.Row) (*ApprovalRule, error) {
	r := &ApprovalRule{}
	err := row.Scan(&r.ID, &r.TeamID, &r.Name, &r.Scope.Type, &r.Scope.ID, &r.Actions, &r.RequiredApprovals, &r.Enabled, &r.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func (s *Store) CreateApprovalRule(ctx context.Context, r *ApprovalRule) (*ApprovalRule, error) {
	return scanRule(s.Pool.QueryRow(ctx, `INSERT INTO approval_rules (team_id, name, scope_type, scope_id, actions, required_approvals, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+ruleCols,
		r.TeamID, r.Name, r.Scope.Type, r.Scope.ID, r.Actions, r.RequiredApprovals, r.Enabled))
}

func (s *Store) UpdateApprovalRule(ctx context.Context, r *ApprovalRule) error {
	_, err := s.Pool.Exec(ctx, `UPDATE approval_rules SET name = $3, actions = $4, required_approvals = $5, enabled = $6,
		updated_at = now() WHERE team_id = $1 AND id = $2`, r.TeamID, r.ID, r.Name, r.Actions, r.RequiredApprovals, r.Enabled)
	return err
}

func (s *Store) DeleteApprovalRule(ctx context.Context, teamID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM approval_rules WHERE team_id = $1 AND id = $2`, teamID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) ApprovalRuleByID(ctx context.Context, teamID, id string) (*ApprovalRule, error) {
	return scanRule(s.Pool.QueryRow(ctx, `SELECT `+ruleCols+` FROM approval_rules WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) ListApprovalRules(ctx context.Context, teamID string) ([]*ApprovalRule, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+ruleCols+` FROM approval_rules WHERE team_id = $1 ORDER BY created_at`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanRule)
}

// MatchingApprovalRules : règles actives couvrant l'action sur l'une des portées de la chaîne.
func (s *Store) MatchingApprovalRules(ctx context.Context, chain []rbac.Scope, action string) ([]*ApprovalRule, error) {
	types := make([]string, len(chain))
	ids := make([]string, len(chain))
	for i, c := range chain {
		types[i], ids[i] = c.Type, c.ID
	}
	rows, err := s.Pool.Query(ctx, `SELECT `+ruleCols+` FROM approval_rules ar
		JOIN unnest($1::text[], $2::text[]) AS c(t, i) ON ar.scope_type = c.t AND ar.scope_id::text = c.i
		WHERE ar.enabled AND $3 = ANY(ar.actions) ORDER BY ar.required_approvals DESC`, types, ids, action)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanRule)
}

type ApprovalVote struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	Decision  string    `json:"decision"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}

type ApprovalRequest struct {
	ID                string          `json:"id"`
	TeamID            string          `json:"team_id"`
	RuleID            *string         `json:"rule_id"`
	RequiredApprovals int             `json:"required_approvals"`
	Action            string          `json:"action"`
	Scope             rbac.Scope      `json:"scope"`
	Summary           string          `json:"summary"`
	Payload           json.RawMessage `json:"payload"`
	RequestedBy       *string         `json:"requested_by"`
	Status            string          `json:"status"`
	Error             string          `json:"error"`
	ExpiresAt         time.Time       `json:"expires_at"`
	DecidedAt         *time.Time      `json:"decided_at"`
	CreatedAt         time.Time       `json:"created_at"`
	Votes             []ApprovalVote  `json:"votes"`
}

const requestSelect = `SELECT ar.id::text, ar.team_id::text, ar.rule_id::text, ar.required_approvals, ar.action, ar.scope_type,
	ar.scope_id::text, ar.summary, ar.payload, ar.requested_by::text, ar.status, ar.error, ar.expires_at, ar.decided_at, ar.created_at,
	coalesce((SELECT json_agg(json_build_object('user_id', v.user_id, 'email', u.email, 'decision', v.decision,
		'comment', v.comment, 'created_at', v.created_at) ORDER BY v.created_at)
	  FROM approval_votes v JOIN users u ON u.id = v.user_id WHERE v.request_id = ar.id), '[]')
	FROM approval_requests ar`

func scanRequest(row pgx.Row) (*ApprovalRequest, error) {
	r := &ApprovalRequest{}
	err := row.Scan(&r.ID, &r.TeamID, &r.RuleID, &r.RequiredApprovals, &r.Action, &r.Scope.Type, &r.Scope.ID, &r.Summary,
		&r.Payload, &r.RequestedBy, &r.Status, &r.Error, &r.ExpiresAt, &r.DecidedAt, &r.CreatedAt, &r.Votes)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

func (r *ApprovalRequest) Approvals() int {
	n := 0
	for _, v := range r.Votes {
		if v.Decision == "approve" {
			n++
		}
	}
	return n
}

func (s *Store) CreateApprovalRequest(ctx context.Context, r *ApprovalRequest) (*ApprovalRequest, error) {
	payload := r.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	var id string
	err := s.Pool.QueryRow(ctx, `INSERT INTO approval_requests (team_id, rule_id, required_approvals, action, scope_type, scope_id,
		summary, payload, requested_by, expires_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10) RETURNING id::text`,
		r.TeamID, r.RuleID, r.RequiredApprovals, r.Action, r.Scope.Type, r.Scope.ID, r.Summary, string(payload),
		r.RequestedBy, r.ExpiresAt).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.ApprovalRequestByID(ctx, id)
}

// ApprovalRequestByID : l'appelant vérifie que la demande appartient à l'équipe (ou une sous-équipe).
func (s *Store) ApprovalRequestByID(ctx context.Context, id string) (*ApprovalRequest, error) {
	return scanRequest(s.Pool.QueryRow(ctx, requestSelect+` WHERE ar.id = $1`, id))
}

func (s *Store) ListApprovalRequests(ctx context.Context, teamIDs []string, status string) ([]*ApprovalRequest, error) {
	if err := s.ExpireApprovalRequests(ctx); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, requestSelect+` WHERE ar.team_id = ANY($1::uuid[]) AND ($2 = '' OR ar.status = $2)
		ORDER BY ar.created_at DESC LIMIT 200`, teamIDs, status)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanRequest)
}

func (s *Store) ExpireApprovalRequests(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE approval_requests SET status = 'expired', decided_at = now()
		WHERE status = 'pending' AND expires_at < now()`)
	return err
}

// AddApprovalVote enregistre un vote unique par utilisateur sur une demande encore en attente.
func (s *Store) AddApprovalVote(ctx context.Context, requestID, userID, decision, comment string) error {
	tag, err := s.Pool.Exec(ctx, `INSERT INTO approval_votes (request_id, user_id, decision, comment)
		SELECT id, $2, $3, $4 FROM approval_requests WHERE id = $1 AND status = 'pending' AND expires_at > now()`,
		requestID, userID, decision, comment)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// TransitionApprovalRequest change le statut si (et seulement si) il vaut from ; renvoie false sinon.
func (s *Store) TransitionApprovalRequest(ctx context.Context, id, from, to, errMsg string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE approval_requests SET status = $3, error = $4, decided_at = coalesce(decided_at, now())
		WHERE id = $1 AND status = $2`, id, from, to, errMsg)
	return tag.RowsAffected() == 1, err
}
