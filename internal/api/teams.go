package api

import (
	"context"
	"errors"
	"net/http"
	"net/mail"
	"slices"
	"strings"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/auth"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const (
	maxTeamDepth  = 5
	invitationTTL = 7 * 24 * time.Hour
)

func (s *Server) listTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := s.Store.UserTeams(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if tok := tokenFrom(r.Context()); tok != nil {
		// Un jeton ne voit que son équipe et ses sous-équipes.
		var visible []*store.Team
		for _, t := range teams {
			chain, err := s.Store.TeamChain(r.Context(), t.ID)
			if err == nil && tok.TeamID != nil && slices.Contains(chain, *tok.TeamID) {
				visible = append(visible, t)
			}
		}
		teams = visible
	}
	writeJSON(w, http.StatusOK, teams)
}

type teamInput struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	ParentID    *string `json:"parent_id"`
}

func (s *Server) createTeam(w http.ResponseWriter, r *http.Request) {
	var in teamInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return
	}
	if tokenFrom(r.Context()) != nil && in.ParentID == nil {
		writeError(w, http.StatusForbidden, "un jeton API ne peut pas créer d'organisation")
		return
	}
	req := r
	if in.ParentID != nil {
		var err error
		if req, err = s.enterTeam(r, *in.ParentID); err != nil {
			s.fail(w, err)
			return
		}
		if !s.authorize(w, req, "team.subteams.manage", rbac.Scope{}) {
			return
		}
		if len(teamFrom(req.Context()).Chain) >= maxTeamDepth {
			writeError(w, http.StatusUnprocessableEntity, "profondeur maximale de sous-équipes atteinte")
			return
		}
	}
	t, err := s.Store.CreateTeam(r.Context(), in.Name, strings.TrimSpace(in.Description), in.ParentID, userFrom(r.Context()).ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(req, &t.ID, "team.created", "team", t.ID, map[string]any{"name": t.Name, "parent_id": t.ParentID})
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) getTeam(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	ta := teamFrom(r.Context())
	perms, owner, err := s.effective(r, rbac.Scope{})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"team": ta.Team, "ancestors": ta.Chain[1:], "permissions": perms, "is_owner": owner})
}

// effective renvoie les permissions effectives de l'utilisateur courant sur la portée.
func (s *Server) effective(r *http.Request, sc rbac.Scope) ([]string, bool, error) {
	ctx := r.Context()
	chain, err := s.scopeChain(ctx, sc)
	if err != nil {
		return nil, false, err
	}
	as, err := s.Store.EffectiveAssignments(ctx, userFrom(ctx).ID, chain)
	if err != nil {
		return nil, false, err
	}
	perms := rbac.Effective(chain, as)
	if tok := tokenFrom(ctx); tok != nil {
		perms = slices.DeleteFunc(perms, func(p string) bool { return !rbac.TokenAllows(tok.Scopes, p) })
	}
	return perms, rbac.IsOwner(chain, as), nil
}

func (s *Server) myPermissions(w http.ResponseWriter, r *http.Request) {
	sc := rbac.Scope{Type: r.URL.Query().Get("scope_type"), ID: r.URL.Query().Get("scope_id")}
	perms, owner, err := s.effective(r, sc)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": perms, "is_owner": owner})
}

func (s *Server) updateTeam(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.update", rbac.Scope{}) {
		return
	}
	var in teamInput
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return
	}
	t := teamFrom(r.Context()).Team
	if err := s.Store.UpdateTeam(r.Context(), t.ID, in.Name, strings.TrimSpace(in.Description)); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.updated", "team", t.ID, map[string]any{
		"before": map[string]string{"name": t.Name, "description": t.Description},
		"after":  map[string]string{"name": in.Name, "description": strings.TrimSpace(in.Description)},
	})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteTeam(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.delete", rbac.Scope{}) {
		return
	}
	t := teamFrom(r.Context()).Team
	if t.Personal {
		writeError(w, http.StatusConflict, "une équipe personnelle ne peut pas être supprimée")
		return
	}
	var in struct {
		DeleteResources bool `json:"delete_resources"`
	}
	if r.ContentLength > 0 && !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	subtree, err := s.Store.TeamSubtree(ctx, t.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	var resources []*store.ResourceSummary
	for _, id := range subtree {
		list, err := s.Store.TeamResources(ctx, id, store.ResourceFilter{})
		if err != nil {
			s.internal(w, err)
			return
		}
		resources = append(resources, list...)
	}
	if len(resources) > 0 && !in.DeleteResources {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "l'équipe (ou une sous-équipe) a encore des ressources : supprimez-les, ou confirmez avec delete_resources pour les arrêter et les effacer des serveurs (volumes compris)",
			"resources": len(resources)})
		return
	}
	if len(resources) > 0 && !s.deployer(w) {
		return
	}
	failed := s.teardownResources(ctx, resources)
	if err := s.Store.DeleteTeam(ctx, t.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.deleted", "team", t.ID, map[string]any{"name": t.Name, "resources": len(resources), "cleanup_failed": failed})
	w.WriteHeader(http.StatusNoContent)
}

// teardownResources arrête et efface des serveurs (volumes compris) puis de la base chaque ressource ;
// un serveur injoignable n'empêche pas la suppression (renvoie les ressources non nettoyées).
func (s *Server) teardownResources(ctx context.Context, list []*store.ResourceSummary) []string {
	failed := []string{}
	for _, res := range list {
		tctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		var err error
		switch res.Type {
		case "application":
			var a *store.Application
			if a, err = s.Store.ApplicationByIDAny(tctx, res.ID); err == nil {
				err = errors.Join(s.Deployer.Teardown(tctx, a, true), s.Store.DeleteApplication(ctx, a.TeamID, a.ID))
			}
		case "database":
			var d *store.Database
			if d, err = s.Store.DatabaseByIDAny(tctx, res.ID); err == nil {
				err = errors.Join(s.Deployer.TeardownDatabase(tctx, d, true), s.Store.DeleteDatabase(ctx, d.TeamID, d.ID))
			}
		case "service":
			var svc *store.Service
			if svc, err = s.Store.ServiceByIDAny(tctx, res.ID); err == nil {
				err = errors.Join(s.Deployer.TeardownService(tctx, svc, true), s.Store.DeleteService(ctx, svc.TeamID, svc.ID))
			}
		}
		cancel()
		if err != nil {
			s.Log.Warn("suppression d'équipe : nettoyage incomplet", "resource", res.ID, "err", err)
			failed = append(failed, res.ID)
		}
	}
	return failed
}

// --- Membres ---

func (s *Server) listMembers(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	members, err := s.Store.Members(r.Context(), teamFrom(r.Context()).Chain)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, members)
}

// addMember ajoute directement à une sous-équipe un membre de l'organisation parente.
func (s *Server) addMember(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	var in struct {
		UserID string `json:"user_id"`
		RoleID string `json:"role_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	ta := teamFrom(ctx)
	if len(ta.Chain) < 2 {
		writeError(w, http.StatusUnprocessableEntity, "utilisez une invitation pour ajouter quelqu'un à une organisation")
		return
	}
	if !isUUID(in.UserID) || !isUUID(in.RoleID) {
		writeError(w, http.StatusUnprocessableEntity, "user_id et role_id requis")
		return
	}
	inParent, err := s.Store.IsMember(ctx, in.UserID, ta.Chain[1:])
	if err != nil {
		s.internal(w, err)
		return
	}
	if !inParent {
		writeError(w, http.StatusUnprocessableEntity, "l'utilisateur doit être membre de l'équipe parente")
		return
	}
	role, ok := s.grantableRole(w, r, in.RoleID, ta.Scope())
	if !ok {
		return
	}
	if err := s.Store.AddMember(ctx, ta.Team.ID, in.UserID); err != nil {
		s.internal(w, err)
		return
	}
	uid := userFrom(ctx).ID
	if _, err := s.Store.CreateAssignment(ctx, &store.RoleAssignment{TeamID: ta.Team.ID, UserID: &in.UserID, RoleID: role.ID,
		Scope: ta.Scope(), Effect: rbac.Allow, CreatedBy: &uid}); err != nil && !errors.Is(err, store.ErrConflict) {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.member.added", "user", in.UserID, map[string]any{"role": role.Name})
	w.WriteHeader(http.StatusNoContent)
}

// grantableRole charge un rôle utilisable dans l'équipe et vérifie que l'appelant peut l'attribuer sur la portée.
func (s *Server) grantableRole(w http.ResponseWriter, r *http.Request, roleID string, sc rbac.Scope) (*store.Role, bool) {
	ctx := r.Context()
	role, err := s.Store.RoleByID(ctx, roleID)
	if err != nil || !role.UsableIn(teamFrom(ctx).Chain) {
		writeError(w, http.StatusUnprocessableEntity, "rôle inconnu")
		return nil, false
	}
	key := ""
	if role.System {
		key = role.Key
	}
	ok, err := s.canGrant(ctx, role.Permissions, key, sc)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "vous ne pouvez pas attribuer un rôle plus large que vos propres droits")
		return nil, false
	}
	return role, true
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	uid, ok := idParam(w, r, "user")
	if !ok {
		return
	}
	s.removeFromTeam(w, r, uid, "team.member.removed")
}

func (s *Server) leaveTeam(w http.ResponseWriter, r *http.Request) {
	if teamFrom(r.Context()).Team.Personal {
		writeError(w, http.StatusConflict, "impossible de quitter son équipe personnelle")
		return
	}
	s.removeFromTeam(w, r, userFrom(r.Context()).ID, "team.member.left")
}

func (s *Server) removeFromTeam(w http.ResponseWriter, r *http.Request, uid, action string) {
	ctx := r.Context()
	ta := teamFrom(ctx)
	direct, err := s.Store.IsDirectMember(ctx, ta.Team.ID, uid)
	if err != nil {
		s.internal(w, err)
		return
	}
	if !direct {
		writeError(w, http.StatusConflict, "ce membre appartient à une équipe parente : retirez-le à ce niveau")
		return
	}
	owners, err := s.Store.OwnerIDs(ctx, ta.Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if slices.Contains(owners, uid) {
		if len(owners) == 1 {
			writeError(w, http.StatusConflict, "impossible de retirer le dernier propriétaire : transférez d'abord la propriété")
			return
		}
		if uid != userFrom(ctx).ID {
			if owner, err := s.isOwner(ctx, rbac.Scope{}); err != nil || !owner {
				writeError(w, http.StatusForbidden, "seul un propriétaire peut retirer un propriétaire")
				return
			}
		}
	}
	if err := s.Store.RemoveMember(ctx, ta.Team.ID, uid); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, action, "user", uid, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) transferOwnership(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID string `json:"user_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	ta := teamFrom(ctx)
	me := userFrom(ctx).ID
	owners, err := s.Store.OwnerIDs(ctx, ta.Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if !slices.Contains(owners, me) || tokenFrom(ctx) != nil {
		writeError(w, http.StatusForbidden, "seul un propriétaire de l'équipe peut transférer la propriété")
		return
	}
	if !isUUID(in.UserID) || in.UserID == me {
		writeError(w, http.StatusUnprocessableEntity, "user_id invalide")
		return
	}
	if ok, err := s.Store.IsMember(ctx, in.UserID, ta.Chain); err != nil || !ok {
		writeError(w, http.StatusUnprocessableEntity, "l'utilisateur doit être membre de l'équipe")
		return
	}
	if err := s.Store.AddMember(ctx, ta.Team.ID, in.UserID); err != nil {
		s.internal(w, err)
		return
	}
	if err := s.Store.TransferOwnership(ctx, ta.Team.ID, me, in.UserID); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.ownership.transferred", "user", in.UserID, map[string]any{"from": me})
	w.WriteHeader(http.StatusNoContent)
}

// --- Invitations ---

func (s *Server) listInvitations(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListInvitations(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	var in struct {
		Email    string   `json:"email"`
		RoleID   string   `json:"role_id"`
		GroupIDs []string `json:"group_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	ta := teamFrom(ctx)
	in.Email = strings.TrimSpace(in.Email)
	if _, err := mail.ParseAddress(in.Email); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "adresse e-mail invalide")
		return
	}
	if !isUUID(in.RoleID) {
		writeError(w, http.StatusUnprocessableEntity, "role_id requis")
		return
	}
	role, ok := s.grantableRole(w, r, in.RoleID, ta.Scope())
	if !ok {
		return
	}
	if in.GroupIDs == nil {
		in.GroupIDs = []string{}
	}
	for _, gid := range in.GroupIDs {
		if !isUUID(gid) {
			writeError(w, http.StatusUnprocessableEntity, "groupe inconnu")
			return
		}
		if _, ok := s.groupGrantable(w, r, gid); !ok {
			return
		}
	}
	tok := auth.NewToken(auth.PrefixInvite)
	uid := userFrom(ctx).ID
	inv, err := s.Store.CreateInvitation(ctx, &store.Invitation{TeamID: ta.Team.ID, Email: in.Email, RoleID: role.ID,
		GroupIDs: in.GroupIDs, TokenHash: auth.HashToken(tok), InvitedBy: &uid, ExpiresAt: time.Now().Add(invitationTTL)})
	if err != nil {
		s.internal(w, err)
		return
	}
	if s.Mailer != nil {
		s.Mailer.TeamInvitation(ctx, in.Email, ta.Team.Name, tok)
	}
	s.auditTeam(r, "team.invitation.created", "invitation", inv.ID, map[string]any{"email": in.Email, "role": role.Name})
	writeJSON(w, http.StatusCreated, map[string]any{"invitation": inv, "token": tok})
}

func (s *Server) deleteInvitation(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	if err := s.Store.DeleteInvitation(r.Context(), teamFrom(r.Context()).Team.ID, id); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "team.invitation.revoked", "invitation", id, nil)
	w.WriteHeader(http.StatusNoContent)
}

// previewInvitation montre l'équipe et le rôle d'une invitation avant de l'accepter.
func (s *Server) previewInvitation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	inv, err := s.Store.InvitationByToken(r.Context(), auth.HashToken(in.Token))
	if err != nil {
		writeError(w, http.StatusNotFound, "invitation invalide ou expirée")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"team_name": inv.TeamName, "role_name": inv.RoleName, "email": inv.Email, "expires_at": inv.ExpiresAt})
}

func (s *Server) acceptInvitation(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	u := userFrom(ctx)
	inv, err := s.Store.InvitationByToken(ctx, auth.HashToken(in.Token))
	if err != nil {
		writeError(w, http.StatusNotFound, "invitation invalide ou expirée")
		return
	}
	if !strings.EqualFold(inv.Email, u.Email) {
		writeError(w, http.StatusForbidden, "cette invitation est destinée à une autre adresse e-mail")
		return
	}
	if err := s.Store.AcceptInvitation(ctx, inv, u.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, &inv.TeamID, "team.invitation.accepted", "invitation", inv.ID, map[string]any{"role": inv.RoleName})
	team, err := s.Store.TeamByID(ctx, inv.TeamID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, team)
}
