package api

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func (s *Server) listPermissions(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"permissions": rbac.Catalog, "scope_types": rbac.ScopeTypes,
		"token_scopes": rbac.TokenScopes, "gateable_actions": rbac.GateableActions})
}

// --- Rôles ---

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	roles, err := s.Store.RolesForTeam(r.Context(), teamFrom(r.Context()).Chain)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, roles)
}

type roleInput struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// validRole valide le rôle et vérifie que l'appelant détient toutes les permissions qu'il accorde.
func (s *Server) validRole(w http.ResponseWriter, r *http.Request, in *roleInput) bool {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 60 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (60 caractères max.)")
		return false
	}
	if len(in.Permissions) == 0 {
		writeError(w, http.StatusUnprocessableEntity, "au moins une permission est requise")
		return false
	}
	for _, p := range in.Permissions {
		if !rbac.ValidGrant(p) {
			writeError(w, http.StatusUnprocessableEntity, "permission inconnue : "+p)
			return false
		}
	}
	slices.Sort(in.Permissions)
	in.Permissions = slices.Compact(in.Permissions)
	ok, err := s.canGrant(r.Context(), in.Permissions, "", rbac.Scope{})
	if err != nil {
		s.fail(w, err)
		return false
	}
	if !ok {
		writeError(w, http.StatusForbidden, "un rôle ne peut pas accorder des permissions que vous n'avez pas")
		return false
	}
	return true
}

func (s *Server) createRole(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.roles.manage", rbac.Scope{}) {
		return
	}
	var in roleInput
	if !decode(w, r, &in) || !s.validRole(w, r, &in) {
		return
	}
	role, err := s.Store.CreateRole(r.Context(), teamFrom(r.Context()).Team.ID, in.Name, strings.TrimSpace(in.Description), in.Permissions)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "team.role.created", "role", role.ID, in)
	writeJSON(w, http.StatusCreated, role)
}

// teamRole charge un rôle personnalisé appartenant à l'équipe courante.
func (s *Server) teamRole(w http.ResponseWriter, r *http.Request) (*store.Role, bool) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	role, err := s.Store.RoleByID(r.Context(), id)
	if err != nil || role.TeamID == nil || *role.TeamID != teamFrom(r.Context()).Team.ID {
		if err == nil && role.System {
			writeError(w, http.StatusForbidden, "les rôles système ne sont pas modifiables")
			return nil, false
		}
		writeError(w, http.StatusNotFound, "rôle introuvable")
		return nil, false
	}
	return role, true
}

func (s *Server) updateRole(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.roles.manage", rbac.Scope{}) {
		return
	}
	role, ok := s.teamRole(w, r)
	if !ok {
		return
	}
	var in roleInput
	if !decode(w, r, &in) || !s.validRole(w, r, &in) {
		return
	}
	if err := s.Store.UpdateRole(r.Context(), role.ID, in.Name, strings.TrimSpace(in.Description), in.Permissions); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "team.role.updated", "role", role.ID, map[string]any{
		"before": roleInput{role.Name, role.Description, role.Permissions}, "after": in})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteRole(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.roles.manage", rbac.Scope{}) {
		return
	}
	role, ok := s.teamRole(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteRole(r.Context(), role.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.role.deleted", "role", role.ID, map[string]any{"name": role.Name})
	w.WriteHeader(http.StatusNoContent)
}

// --- Attributions ---

func (s *Server) listAssignments(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListAssignments(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createAssignment(w http.ResponseWriter, r *http.Request) {
	var in struct {
		UserID  *string    `json:"user_id"`
		GroupID *string    `json:"group_id"`
		RoleID  string     `json:"role_id"`
		Scope   rbac.Scope `json:"scope"`
		Effect  string     `json:"effect"`
	}
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	ta := teamFrom(ctx)
	if in.Scope.Type == "" {
		in.Scope = ta.Scope()
	}
	// La gestion des droits peut être déléguée sur un projet, un serveur… : on vérifie sur la portée visée.
	if !s.authorize(w, r, "team.roles.manage", in.Scope) {
		return
	}
	if in.Effect == "" {
		in.Effect = rbac.Allow
	}
	if in.Effect != rbac.Allow && in.Effect != rbac.Deny {
		writeError(w, http.StatusUnprocessableEntity, "effect doit valoir allow ou deny")
		return
	}
	if (in.UserID == nil) == (in.GroupID == nil) {
		writeError(w, http.StatusUnprocessableEntity, "indiquez user_id ou group_id")
		return
	}
	if !isUUID(in.RoleID) {
		writeError(w, http.StatusUnprocessableEntity, "role_id requis")
		return
	}
	role, ok := s.grantableRole(w, r, in.RoleID, in.Scope)
	if !ok {
		return
	}
	if role.System && role.Key == rbac.RoleOwner && (in.GroupID != nil || in.Scope != ta.Scope() || in.Effect != rbac.Allow) {
		writeError(w, http.StatusUnprocessableEntity, "le rôle propriétaire s'attribue uniquement à un utilisateur, sur l'équipe")
		return
	}
	if in.UserID != nil {
		member, err := s.Store.IsMember(ctx, *in.UserID, ta.Chain)
		if err != nil || !member {
			writeError(w, http.StatusUnprocessableEntity, "l'utilisateur n'est pas membre de l'équipe")
			return
		}
	} else if _, err := s.Store.GroupInChain(ctx, ta.Chain, *in.GroupID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "groupe inconnu")
		return
	}
	uid := userFrom(ctx).ID
	a, err := s.Store.CreateAssignment(ctx, &store.RoleAssignment{TeamID: ta.Team.ID, UserID: in.UserID, GroupID: in.GroupID,
		RoleID: role.ID, Scope: in.Scope, Effect: in.Effect, CreatedBy: &uid})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "team.role_assignment.created", "role_assignment", a.ID, a)
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) deleteAssignment(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	ctx := r.Context()
	ta := teamFrom(ctx)
	a, err := s.Store.AssignmentByID(ctx, ta.Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !s.authorize(w, r, "team.roles.manage", a.Scope) {
		return
	}
	role, err := s.Store.RoleByID(ctx, a.RoleID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if role.System && role.Key == rbac.RoleOwner {
		owners, err := s.Store.OwnerIDs(ctx, ta.Team.ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		if len(owners) <= 1 {
			writeError(w, http.StatusConflict, "impossible de retirer le dernier propriétaire")
			return
		}
		if owner, err := s.isOwner(ctx, rbac.Scope{}); err != nil || !owner {
			writeError(w, http.StatusForbidden, "seul un propriétaire peut retirer un propriétaire")
			return
		}
	}
	if err := s.Store.DeleteAssignment(ctx, ta.Team.ID, id); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.role_assignment.deleted", "role_assignment", id, a)
	w.WriteHeader(http.StatusNoContent)
}

// --- Groupes ---

func (s *Server) listGroups(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListGroups(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type groupInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (in *groupInput) valid(w http.ResponseWriter) bool {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 60 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (60 caractères max.)")
		return false
	}
	return true
}

func (s *Server) createGroup(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	var in groupInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	g, err := s.Store.CreateGroup(r.Context(), teamFrom(r.Context()).Team.ID, in.Name, strings.TrimSpace(in.Description))
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "team.group.created", "group", g.ID, in)
	writeJSON(w, http.StatusCreated, g)
}

// teamGroup charge un groupe défini par l'équipe courante.
func (s *Server) teamGroup(w http.ResponseWriter, r *http.Request) (*store.Group, bool) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	g, err := s.Store.GroupInChain(r.Context(), []string{teamFrom(r.Context()).Team.ID}, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return g, true
}

func (s *Server) updateGroup(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	g, ok := s.teamGroup(w, r)
	if !ok {
		return
	}
	var in groupInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	if err := s.Store.UpdateGroup(r.Context(), g.ID, in.Name, strings.TrimSpace(in.Description)); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "team.group.updated", "group", g.ID, map[string]any{"before": groupInput{g.Name, g.Description}, "after": in})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteGroup(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	g, ok := s.teamGroup(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteGroup(r.Context(), g.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.group.deleted", "group", g.ID, map[string]any{"name": g.Name})
	w.WriteHeader(http.StatusNoContent)
}

// groupGrantable vérifie que l'appelant détient tous les droits que le groupe confère
// (sinon ajouter quelqu'un, y compris soi-même, au groupe serait une élévation de privilèges).
func (s *Server) groupGrantable(w http.ResponseWriter, r *http.Request, groupID string) (*store.Group, bool) {
	ctx := r.Context()
	ta := teamFrom(ctx)
	g, err := s.Store.GroupInChain(ctx, ta.Chain, groupID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "groupe inconnu")
		return nil, false
	}
	list, err := s.Store.ListAssignments(ctx, ta.Team.ID)
	if err != nil {
		s.internal(w, err)
		return nil, false
	}
	for _, a := range list {
		if a.GroupID == nil || *a.GroupID != g.ID || a.Effect != rbac.Allow {
			continue
		}
		role, err := s.Store.RoleByID(ctx, a.RoleID)
		if err != nil {
			s.internal(w, err)
			return nil, false
		}
		ok, err := s.canGrant(ctx, role.Permissions, "", a.Scope)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			s.internal(w, err)
			return nil, false
		}
		if !ok {
			writeError(w, http.StatusForbidden, "ce groupe confère des droits que vous n'avez pas")
			return nil, false
		}
	}
	return g, true
}

func (s *Server) addGroupMember(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	g, ok := s.teamGroup(w, r)
	if !ok {
		return
	}
	uid, ok := idParam(w, r, "user")
	if !ok {
		return
	}
	ctx := r.Context()
	if member, err := s.Store.IsMember(ctx, uid, teamFrom(ctx).Chain); err != nil || !member {
		writeError(w, http.StatusUnprocessableEntity, "l'utilisateur n'est pas membre de l'équipe")
		return
	}
	if _, ok := s.groupGrantable(w, r, g.ID); !ok {
		return
	}
	if err := s.Store.AddGroupMember(ctx, g.ID, uid); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.group.member_added", "group", g.ID, map[string]any{"user_id": uid})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeGroupMember(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.members.manage", rbac.Scope{}) {
		return
	}
	g, ok := s.teamGroup(w, r)
	if !ok {
		return
	}
	uid, ok := idParam(w, r, "user")
	if !ok {
		return
	}
	if err := s.Store.RemoveGroupMember(r.Context(), g.ID, uid); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "team.group.member_removed", "group", g.ID, map[string]any{"user_id": uid})
	w.WriteHeader(http.StatusNoContent)
}
