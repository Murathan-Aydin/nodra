package api

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// auditFilter lit les filtres communs (équipe + sous-équipes) depuis la query string.
func (s *Server) auditFilter(w http.ResponseWriter, r *http.Request, maxLimit int) (store.AuditFilter, bool) {
	q := r.URL.Query()
	subtree, err := s.Store.TeamSubtree(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return store.AuditFilter{}, false
	}
	f := store.AuditFilter{TeamIDs: subtree, ActorUserID: q.Get("actor"), Action: q.Get("action"),
		TargetType: q.Get("target_type"), TargetID: q.Get("target_id"), Limit: maxLimit}
	if f.ActorUserID != "" && !isUUID(f.ActorUserID) {
		writeError(w, http.StatusBadRequest, "actor invalide")
		return f, false
	}
	for name, dst := range map[string]**time.Time{"since": &f.Since, "until": &f.Until} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeError(w, http.StatusBadRequest, name+" doit être une date RFC 3339")
				return f, false
			}
			*dst = &t
		}
	}
	if v := q.Get("before"); v != "" {
		f.Before, _ = strconv.ParseInt(v, 10, 64)
	}
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n < maxLimit {
			f.Limit = n
		}
	}
	return f, true
}

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.audit.read", rbac.Scope{}) {
		return
	}
	f, ok := s.auditFilter(w, r, 200)
	if !ok {
		return
	}
	if r.URL.Query().Get("limit") == "" {
		f.Limit = 50
	}
	events, err := s.Store.ListAudit(r.Context(), f)
	if err != nil {
		s.internal(w, err)
		return
	}
	var next *int64
	if len(events) == f.Limit {
		next = &events[len(events)-1].ID
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "next_before": next})
}

func (s *Server) exportAudit(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.audit.read", rbac.Scope{}) {
		return
	}
	f, ok := s.auditFilter(w, r, 10000)
	if !ok {
		return
	}
	events, err := s.Store.ListAudit(r.Context(), f)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.audit.exported", "team", teamFrom(r.Context()).Team.ID, map[string]any{"count": len(events)})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="nodra-audit.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"id", "date", "team_id", "actor_email", "actor_user_id", "actor_token_id", "action",
		"target_type", "target_id", "ip", "user_agent", "metadata"})
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	for _, e := range events {
		_ = cw.Write([]string{strconv.FormatInt(e.ID, 10), e.CreatedAt.UTC().Format(time.RFC3339), str(e.TeamID), e.ActorEmail,
			str(e.ActorUserID), str(e.ActorTokenID), e.Action, e.TargetType, e.TargetID, e.IP, e.UserAgent, string(e.Metadata)})
	}
	cw.Flush()
}
