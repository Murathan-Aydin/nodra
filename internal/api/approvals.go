package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// --- Règles ---

func (s *Server) listApprovalRules(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	rules, err := s.Store.ListApprovalRules(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rules)
}

type ruleInput struct {
	Name              string     `json:"name"`
	Scope             rbac.Scope `json:"scope"`
	Actions           []string   `json:"actions"`
	RequiredApprovals int        `json:"required_approvals"`
	Enabled           *bool      `json:"enabled"`
}

func (in *ruleInput) valid(w http.ResponseWriter) bool {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return false
	}
	if len(in.Actions) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "au moins une action est requise")
		return false
	}
	for _, a := range in.Actions {
		if !slices.Contains(rbac.GateableActions, a) {
			writeError(w, http.StatusUnprocessableEntity, "action non soumissible à approbation : "+a)
			return false
		}
	}
	if in.RequiredApprovals == 0 {
		in.RequiredApprovals = 1
	}
	if in.RequiredApprovals < 1 || in.RequiredApprovals > 10 {
		writeError(w, http.StatusUnprocessableEntity, "required_approvals doit être entre 1 et 10")
		return false
	}
	if in.Enabled == nil {
		t := true
		in.Enabled = &t
	}
	return true
}

func (s *Server) createApprovalRule(w http.ResponseWriter, r *http.Request) {
	var in ruleInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	ta := teamFrom(r.Context())
	if in.Scope.Type == "" {
		in.Scope = ta.Scope()
	}
	if !s.authorize(w, r, "approvals.manage", in.Scope) {
		return
	}
	rule, err := s.Store.CreateApprovalRule(r.Context(), &store.ApprovalRule{TeamID: ta.Team.ID, Name: in.Name, Scope: in.Scope,
		Actions: in.Actions, RequiredApprovals: in.RequiredApprovals, Enabled: *in.Enabled})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "approval.rule.created", "approval_rule", rule.ID, rule)
	writeJSON(w, http.StatusCreated, rule)
}

func (s *Server) teamApprovalRule(w http.ResponseWriter, r *http.Request) (*store.ApprovalRule, bool) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	rule, err := s.Store.ApprovalRuleByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return rule, s.authorize(w, r, "approvals.manage", rule.Scope)
}

func (s *Server) updateApprovalRule(w http.ResponseWriter, r *http.Request) {
	rule, ok := s.teamApprovalRule(w, r)
	if !ok {
		return
	}
	var in ruleInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	before := *rule
	rule.Name, rule.Actions, rule.RequiredApprovals, rule.Enabled = in.Name, in.Actions, in.RequiredApprovals, *in.Enabled
	if err := s.Store.UpdateApprovalRule(r.Context(), rule); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "approval.rule.updated", "approval_rule", rule.ID, map[string]any{"before": before, "after": rule})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteApprovalRule(w http.ResponseWriter, r *http.Request) {
	rule, ok := s.teamApprovalRule(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteApprovalRule(r.Context(), rule.TeamID, rule.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "approval.rule.deleted", "approval_rule", rule.ID, rule)
	w.WriteHeader(http.StatusNoContent)
}

// --- Demandes ---

func (s *Server) listApprovalRequests(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	subtree, err := s.Store.TeamSubtree(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	list, err := s.Store.ListApprovalRequests(r.Context(), subtree, r.URL.Query().Get("status"))
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// teamApprovalRequest charge une demande de l'équipe courante ou d'une de ses sous-équipes.
func (s *Server) teamApprovalRequest(w http.ResponseWriter, r *http.Request) (*store.ApprovalRequest, bool) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	ctx := r.Context()
	if err := s.Store.ExpireApprovalRequests(ctx); err != nil {
		s.internal(w, err)
		return nil, false
	}
	req, err := s.Store.ApprovalRequestByID(ctx, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	subtree, err := s.Store.TeamSubtree(ctx, teamFrom(ctx).Team.ID)
	if err != nil {
		s.internal(w, err)
		return nil, false
	}
	if !slices.Contains(subtree, req.TeamID) {
		writeError(w, http.StatusNotFound, "introuvable")
		return nil, false
	}
	return req, true
}

func (s *Server) getApprovalRequest(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	if req, ok := s.teamApprovalRequest(w, r); ok {
		writeJSON(w, http.StatusOK, req)
	}
}

func (s *Server) voteApproval(decision string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		req, ok := s.teamApprovalRequest(w, r)
		if !ok {
			return
		}
		var in struct {
			Comment string `json:"comment"`
		}
		if r.ContentLength > 0 && !decode(w, r, &in) {
			return
		}
		ctx := r.Context()
		me := userFrom(ctx).ID
		if req.RequestedBy != nil && *req.RequestedBy == me {
			writeError(w, http.StatusForbidden, "vous ne pouvez pas valider votre propre demande")
			return
		}
		if !s.authorize(w, r, "approvals.approve", req.Scope) {
			return
		}
		err := s.Store.AddApprovalVote(ctx, req.ID, me, decision, truncate(strings.TrimSpace(in.Comment), 1000))
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "vous avez déjà voté")
			return
		} else if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusConflict, "la demande n'est plus en attente")
			return
		} else if err != nil {
			s.internal(w, err)
			return
		}
		s.audit(r, &req.TeamID, "approval."+decision+"d", "approval_request", req.ID, map[string]any{"comment": in.Comment})
		if req, err = s.Store.ApprovalRequestByID(ctx, req.ID); err != nil {
			s.internal(w, err)
			return
		}
		switch {
		case decision == "reject":
			if _, err := s.Store.TransitionApprovalRequest(ctx, req.ID, "pending", "rejected", ""); err != nil {
				s.internal(w, err)
				return
			}
		case req.Approvals() >= req.RequiredApprovals:
			// Une seule requête gagne la transition, même si deux approbateurs votent en même temps.
			won, err := s.Store.TransitionApprovalRequest(ctx, req.ID, "pending", "approved", "")
			if err != nil {
				s.internal(w, err)
				return
			}
			if won {
				s.execute(r, req)
			}
		}
		req, err = s.Store.ApprovalRequestByID(ctx, req.ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		writeJSON(w, http.StatusOK, req)
	}
}

func (s *Server) cancelApproval(w http.ResponseWriter, r *http.Request) {
	req, ok := s.teamApprovalRequest(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if req.RequestedBy == nil || *req.RequestedBy != userFrom(ctx).ID {
		if !s.authorize(w, r, "approvals.manage", req.Scope) {
			return
		}
	}
	won, err := s.Store.TransitionApprovalRequest(ctx, req.ID, "pending", "cancelled", "")
	if err != nil {
		s.internal(w, err)
		return
	}
	if !won {
		writeError(w, http.StatusConflict, "la demande n'est plus en attente")
		return
	}
	s.audit(r, &req.TeamID, "approval.cancelled", "approval_request", req.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}
