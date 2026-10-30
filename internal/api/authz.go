package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const ctxTeam ctxKey = 100

// teamAccess : équipe courante (paramètre {team} de l'URL) et sa chaîne d'ancêtres.
type teamAccess struct {
	Team  *store.Team
	Chain []string // équipe → organisation racine
}

func (ta *teamAccess) Scope() rbac.Scope { return rbac.Scope{Type: rbac.ScopeTeam, ID: ta.Team.ID} }

func teamFrom(ctx context.Context) *teamAccess {
	t, _ := ctx.Value(ctxTeam).(*teamAccess)
	return t
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func isUUID(s string) bool { return uuidRe.MatchString(s) }

// idParam lit un identifiant UUID de l'URL ; répond 404 s'il est malformé.
func idParam(w http.ResponseWriter, r *http.Request, name string) (string, bool) {
	id := chi.URLParam(r, name)
	if !isUUID(id) {
		writeError(w, http.StatusNotFound, "introuvable")
		return "", false
	}
	return id, true
}

// enterTeam vérifie l'appartenance de l'utilisateur (et du jeton API) à l'équipe et l'ajoute au contexte.
// Une équipe dont l'utilisateur n'est pas membre est signalée comme introuvable.
func (s *Server) enterTeam(r *http.Request, id string) (*http.Request, error) {
	ctx := r.Context()
	if !isUUID(id) {
		return nil, store.ErrNotFound
	}
	team, err := s.Store.TeamByID(ctx, id)
	if err != nil {
		return nil, err
	}
	chain, err := s.Store.TeamChain(ctx, id)
	if err != nil {
		return nil, err
	}
	ok, err := s.Store.IsMember(ctx, userFrom(ctx).ID, chain)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, store.ErrNotFound
	}
	if tok := tokenFrom(ctx); tok != nil && (tok.TeamID == nil || !slices.Contains(chain, *tok.TeamID)) {
		return nil, errForbidden
	}
	return r.WithContext(context.WithValue(ctx, ctxTeam, &teamAccess{Team: team, Chain: chain})), nil
}

var errForbidden = errors.New("accès refusé")

func (s *Server) withTeam(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r2, err := s.enterTeam(r, chi.URLParam(r, "team"))
		if err != nil {
			s.fail(w, err)
			return
		}
		next.ServeHTTP(w, r2)
	})
}

// scopeChain renvoie la chaîne de la portée et vérifie qu'elle appartient à l'équipe courante
// (ou à l'une de ses sous-équipes). Portée vide = l'équipe courante.
func (s *Server) scopeChain(ctx context.Context, sc rbac.Scope) ([]rbac.Scope, error) {
	ta := teamFrom(ctx)
	if sc.Type == "" || sc == ta.Scope() {
		out := make([]rbac.Scope, len(ta.Chain))
		for i, id := range ta.Chain {
			out[i] = rbac.Scope{Type: rbac.ScopeTeam, ID: id}
		}
		return out, nil
	}
	if !slices.Contains(rbac.ScopeTypes, sc.Type) || !isUUID(sc.ID) {
		return nil, store.ErrNotFound
	}
	chain, err := s.Store.ScopeChain(ctx, sc)
	if err != nil {
		return nil, err
	}
	if !slices.Contains(chain, ta.Scope()) {
		return nil, store.ErrNotFound
	}
	return chain, nil
}

// can indique si l'utilisateur courant (et son jeton éventuel) a la permission sur la portée.
func (s *Server) can(ctx context.Context, perm string, sc rbac.Scope) (bool, error) {
	if tok := tokenFrom(ctx); tok != nil && !rbac.TokenAllows(tok.Scopes, perm) {
		return false, nil
	}
	chain, err := s.scopeChain(ctx, sc)
	if err != nil {
		return false, err
	}
	as, err := s.Store.EffectiveAssignments(ctx, userFrom(ctx).ID, chain)
	if err != nil {
		return false, err
	}
	return rbac.Decide(chain, as, perm), nil
}

// authorize répond 403 (ou 404/500) et renvoie false si la permission manque.
func (s *Server) authorize(w http.ResponseWriter, r *http.Request, perm string, sc rbac.Scope) bool {
	ok, err := s.can(r.Context(), perm, sc)
	if err != nil {
		s.fail(w, err)
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "permission requise : "+perm)
		return false
	}
	return true
}

// canGrant : on ne peut attribuer un rôle que si l'on détient soi-même toutes ses permissions sur la portée.
// Le rôle propriétaire n'est attribuable que par un propriétaire.
func (s *Server) canGrant(ctx context.Context, perms []string, systemKey string, sc rbac.Scope) (bool, error) {
	chain, err := s.scopeChain(ctx, sc)
	if err != nil {
		return false, err
	}
	as, err := s.Store.EffectiveAssignments(ctx, userFrom(ctx).ID, chain)
	if err != nil {
		return false, err
	}
	if rbac.IsOwner(chain, as) {
		return true, nil
	}
	if systemKey == rbac.RoleOwner {
		return false, nil
	}
	for _, p := range rbac.Expand(perms) {
		if !rbac.Decide(chain, as, p) {
			return false, nil
		}
	}
	return true, nil
}

func (s *Server) isOwner(ctx context.Context, sc rbac.Scope) (bool, error) {
	chain, err := s.scopeChain(ctx, sc)
	if err != nil {
		return false, err
	}
	as, err := s.Store.EffectiveAssignments(ctx, userFrom(ctx).ID, chain)
	if err != nil {
		return false, err
	}
	return rbac.IsOwner(chain, as), nil
}

// --- Audit ---

// audit enregistre un événement au nom de l'utilisateur courant. Un échec est journalisé sans faire échouer la requête.
func (s *Server) audit(r *http.Request, teamID *string, action, targetType, targetID string, meta any) {
	ctx := r.Context()
	e := &store.AuditEvent{TeamID: teamID, Action: action, TargetType: targetType, TargetID: targetID,
		IP: clientIP(r), UserAgent: truncate(r.UserAgent(), 255)}
	if u := userFrom(ctx); u != nil {
		e.ActorUserID, e.ActorEmail = &u.ID, u.Email
	}
	if t := tokenFrom(ctx); t != nil {
		e.ActorTokenID = &t.ID
	}
	if meta != nil {
		e.Metadata, _ = json.Marshal(meta)
	}
	if err := s.Store.InsertAudit(context.WithoutCancel(ctx), e); err != nil {
		s.Log.Error("audit", "action", action, "err", err)
	}
}

// auditTeam : événement rattaché à l'équipe courante.
func (s *Server) auditTeam(r *http.Request, action, targetType, targetID string, meta any) {
	id := teamFrom(r.Context()).Team.ID
	s.audit(r, &id, action, targetType, targetID, meta)
}

// --- Approbations ---

// Executor exécute une action approuvée (enregistré par chaque module : déploiement, suppression…).
type Executor func(ctx context.Context, req *store.ApprovalRequest) error

const approvalTTL = 7 * 24 * time.Hour

// gate soumet l'action à approbation si une règle active la couvre.
// Renvoie la demande créée ; nil signifie que l'action peut être exécutée immédiatement.
func (s *Server) gate(r *http.Request, action string, sc rbac.Scope, summary string, payload any) (*store.ApprovalRequest, error) {
	uid := userFrom(r.Context()).ID
	return s.gateAs(r, teamFrom(r.Context()).Team.ID, &uid, action, sc, summary, payload)
}

// gateAs : comme gate, pour un demandeur explicite (nil pour un webhook) et hors contexte d'équipe.
func (s *Server) gateAs(r *http.Request, teamID string, requestedBy *string, action string, sc rbac.Scope, summary string, payload any) (*store.ApprovalRequest, error) {
	ctx := r.Context()
	scopeChain := s.scopeChain
	if teamFrom(ctx) == nil { // webhook : la portée a été résolue par l'appelant
		scopeChain = s.Store.ScopeChain
	}
	chain, err := scopeChain(ctx, sc)
	if err != nil {
		return nil, err
	}
	rules, err := s.Store.MatchingApprovalRules(ctx, chain, action)
	if err != nil || len(rules) == 0 {
		return nil, err
	}
	rule := rules[0] // la plus exigeante
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := s.Store.CreateApprovalRequest(ctx, &store.ApprovalRequest{
		TeamID: teamID, RuleID: &rule.ID, RequiredApprovals: rule.RequiredApprovals,
		Action: action, Scope: chain[0], Summary: summary, Payload: b, RequestedBy: requestedBy,
		ExpiresAt: time.Now().Add(approvalTTL),
	})
	if err != nil {
		return nil, err
	}
	s.audit(r, &teamID, "approval.requested", "approval_request", req.ID, map[string]any{"action": action, "scope": chain[0], "summary": summary})
	return req, nil
}

// respondPending : réponse standard quand une action attend une approbation.
func respondPending(w http.ResponseWriter, req *store.ApprovalRequest) {
	writeJSON(w, http.StatusAccepted, map[string]any{"approval_required": true, "approval_request": req})
}

// execute lance l'exécuteur d'une demande approuvée et enregistre le résultat.
func (s *Server) execute(r *http.Request, req *store.ApprovalRequest) {
	ctx := context.WithoutCancel(r.Context())
	exec := s.Executors[req.Action]
	status, msg := "executed", ""
	if exec == nil {
		status, msg = "failed", "aucun exécuteur pour l'action "+req.Action
	} else if err := exec(ctx, req); err != nil {
		status, msg = "failed", err.Error()
	}
	if _, err := s.Store.TransitionApprovalRequest(ctx, req.ID, "approved", status, msg); err != nil {
		s.Log.Error("approbation", "id", req.ID, "err", err)
	}
	s.audit(r, &req.TeamID, "approval."+status, "approval_request", req.ID, map[string]any{"action": req.Action, "error": msg})
}

// --- Erreurs ---

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "introuvable")
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "existe déjà")
	case errors.Is(err, store.ErrInUse):
		writeError(w, http.StatusConflict, "encore utilisé par d'autres éléments")
	case errors.Is(err, errForbidden):
		writeError(w, http.StatusForbidden, "accès refusé")
	default:
		s.internal(w, err)
	}
}
