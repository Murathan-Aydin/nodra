package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

var (
	envKeyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,254}$`)
	colorRe  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

type namedInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (in *namedInput) valid(w http.ResponseWriter) bool {
	in.Name, in.Description = strings.TrimSpace(in.Name), strings.TrimSpace(in.Description)
	if in.Name == "" || len(in.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return false
	}
	return true
}

func projectScope(id string) rbac.Scope { return rbac.Scope{Type: rbac.ScopeProject, ID: id} }
func envScope(id string) rbac.Scope     { return rbac.Scope{Type: rbac.ScopeEnvironment, ID: id} }

// --- Projets ---

// listProjects : projets visibles par l'utilisateur (droit project.read sur l'équipe ou sur le projet).
func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	list, err := s.Store.ListProjects(ctx, teamFrom(ctx).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	visible := []*store.Project{}
	for _, p := range list {
		if ok, err := s.can(ctx, "project.read", projectScope(p.ID)); err != nil {
			s.internal(w, err)
			return
		} else if ok {
			visible = append(visible, p)
		}
	}
	writeJSON(w, http.StatusOK, visible)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "project.create", rbac.Scope{}) {
		return
	}
	var in namedInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	p, err := s.Store.CreateProject(r.Context(), teamFrom(r.Context()).Team.ID, in.Name, in.Description)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "project.created", "project", p.ID, in)
	writeJSON(w, http.StatusCreated, p)
}

// teamProject charge le projet {project} de l'équipe et vérifie la permission sur sa portée.
func (s *Server) teamProject(w http.ResponseWriter, r *http.Request, perm string) (*store.Project, bool) {
	id, ok := idParam(w, r, "project")
	if !ok {
		return nil, false
	}
	p, err := s.Store.ProjectByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	if perm == "" {
		return p, true
	}
	return p, s.authorize(w, r, perm, projectScope(p.ID))
}

func (s *Server) getProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.teamProject(w, r, "")
	if !ok {
		return
	}
	ctx := r.Context()
	envs, err := s.Store.ListEnvironments(ctx, p.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	// Visible si l'utilisateur peut lire le projet ou au moins un de ses environnements.
	visible := []*store.Environment{}
	for _, e := range envs {
		if ok, err := s.can(ctx, "project.read", envScope(e.ID)); err != nil {
			s.internal(w, err)
			return
		} else if ok {
			visible = append(visible, e)
		}
	}
	if len(visible) == 0 && !s.authorize(w, r, "project.read", projectScope(p.ID)) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": p, "environments": visible})
}

func (s *Server) updateProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.teamProject(w, r, "project.update")
	if !ok {
		return
	}
	var in namedInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	if err := s.Store.UpdateProject(r.Context(), p.TeamID, p.ID, in.Name, in.Description); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "project.updated", "project", p.ID, map[string]any{"before": namedInput{p.Name, p.Description}, "after": in})
	w.WriteHeader(http.StatusNoContent)
}

type deletePayload struct {
	TeamID    string `json:"team_id"`
	ProjectID string `json:"project_id"`
	EnvID     string `json:"environment_id,omitempty"`
}

func (s *Server) deleteProject(w http.ResponseWriter, r *http.Request) {
	p, ok := s.teamProject(w, r, "project.delete")
	if !ok {
		return
	}
	req, err := s.gate(r, "project.delete", projectScope(p.ID), "Suppression du projet "+p.Name, deletePayload{TeamID: p.TeamID, ProjectID: p.ID})
	if err != nil {
		s.fail(w, err)
		return
	}
	if req != nil {
		respondPending(w, req)
		return
	}
	if err := s.Store.DeleteProject(r.Context(), p.TeamID, p.ID); err != nil {
		s.inUse(w, err, "le projet contient encore des ressources")
		return
	}
	s.auditTeam(r, "project.deleted", "project", p.ID, map[string]string{"name": p.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) execDeleteProject(ctx context.Context, req *store.ApprovalRequest) error {
	var p deletePayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	if err := s.Store.DeleteProject(ctx, p.TeamID, p.ProjectID); errors.Is(err, store.ErrInUse) {
		return errors.New("le projet contient encore des ressources")
	} else {
		return err
	}
}

// inUse répond 409 avec un message explicite pour ErrInUse, sinon traite l'erreur normalement.
func (s *Server) inUse(w http.ResponseWriter, err error, msg string) {
	if errors.Is(err, store.ErrInUse) {
		writeError(w, http.StatusConflict, msg)
		return
	}
	s.fail(w, err)
}

// --- Environnements ---

func (s *Server) createEnvironment(w http.ResponseWriter, r *http.Request) {
	p, ok := s.teamProject(w, r, "environment.create")
	if !ok {
		return
	}
	var in namedInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	e, err := s.Store.CreateEnvironment(r.Context(), p.ID, in.Name, in.Description)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "environment.created", "environment", e.ID, map[string]string{"project_id": p.ID, "name": e.Name})
	writeJSON(w, http.StatusCreated, e)
}

// projectEnv charge l'environnement {env} du projet {project} et vérifie la permission sur l'environnement.
func (s *Server) projectEnv(w http.ResponseWriter, r *http.Request, perm string) (*store.Project, *store.Environment, bool) {
	p, ok := s.teamProject(w, r, "")
	if !ok {
		return nil, nil, false
	}
	id, ok := idParam(w, r, "env")
	if !ok {
		return nil, nil, false
	}
	e, err := s.Store.EnvironmentByID(r.Context(), p.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, nil, false
	}
	return p, e, s.authorize(w, r, perm, envScope(e.ID))
}

func (s *Server) getEnvironment(w http.ResponseWriter, r *http.Request) {
	p, e, ok := s.projectEnv(w, r, "project.read")
	if ok {
		writeJSON(w, http.StatusOK, map[string]any{"project": p, "environment": e})
	}
}

func (s *Server) updateEnvironment(w http.ResponseWriter, r *http.Request) {
	p, e, ok := s.projectEnv(w, r, "environment.update")
	if !ok {
		return
	}
	var in namedInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	if err := s.Store.UpdateEnvironment(r.Context(), p.ID, e.ID, in.Name, in.Description); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "environment.updated", "environment", e.ID, map[string]any{"before": namedInput{e.Name, e.Description}, "after": in})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteEnvironment(w http.ResponseWriter, r *http.Request) {
	p, e, ok := s.projectEnv(w, r, "environment.delete")
	if !ok {
		return
	}
	req, err := s.gate(r, "environment.delete", envScope(e.ID), "Suppression de l'environnement "+p.Name+" / "+e.Name,
		deletePayload{TeamID: p.TeamID, ProjectID: p.ID, EnvID: e.ID})
	if err != nil {
		s.fail(w, err)
		return
	}
	if req != nil {
		respondPending(w, req)
		return
	}
	if err := s.Store.DeleteEnvironment(r.Context(), p.ID, e.ID); err != nil {
		s.inUse(w, err, "l'environnement contient encore des ressources")
		return
	}
	s.auditTeam(r, "environment.deleted", "environment", e.ID, map[string]string{"project_id": p.ID, "name": e.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) execDeleteEnvironment(ctx context.Context, req *store.ApprovalRequest) error {
	var p deletePayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	if err := s.Store.DeleteEnvironment(ctx, p.ProjectID, p.EnvID); errors.Is(err, store.ErrInUse) {
		return errors.New("l'environnement contient encore des ressources")
	} else {
		return err
	}
}

// cloneEnvironment copie l'environnement (vers le même projet ou un autre projet de l'équipe).
func (s *Server) cloneEnvironment(w http.ResponseWriter, r *http.Request) {
	p, e, ok := s.projectEnv(w, r, "project.read")
	if !ok {
		return
	}
	var in struct {
		Name      string `json:"name"`
		ProjectID string `json:"project_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return
	}
	target := p
	if in.ProjectID != "" && in.ProjectID != p.ID {
		if !isUUID(in.ProjectID) {
			writeError(w, http.StatusUnprocessableEntity, "projet cible inconnu")
			return
		}
		var err error
		if target, err = s.Store.ProjectByID(r.Context(), p.TeamID, in.ProjectID); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "projet cible inconnu")
			return
		}
	}
	if !s.authorize(w, r, "environment.create", projectScope(target.ID)) {
		return
	}
	clone, err := s.Store.CloneEnvironment(r.Context(), e, target.ID, in.Name)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "environment.cloned", "environment", clone.ID, map[string]string{"source_id": e.ID, "project_id": target.ID})
	writeJSON(w, http.StatusCreated, clone)
}

// --- Variables partagées ---

var sharedScopes = []string{rbac.ScopeTeam, rbac.ScopeProject, rbac.ScopeEnvironment, rbac.ScopeServer}

// revealVar déchiffre la valeur, sauf pour un secret.
func (s *Server) revealVar(v *store.SharedVariable) error {
	if v.IsSecret {
		v.Value = ""
		return nil
	}
	var err error
	v.Value, err = s.Box.Decrypt(v.ValueEnc)
	return err
}

func (s *Server) listVariables(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sc := rbac.Scope{Type: r.URL.Query().Get("scope_type"), ID: r.URL.Query().Get("scope_id")}
	if sc.Type == "" {
		sc = teamFrom(ctx).Scope()
	}
	if !slices.Contains(sharedScopes, sc.Type) {
		writeError(w, http.StatusUnprocessableEntity, "scope_type : team, project, environment ou server")
		return
	}
	if !s.authorize(w, r, "variable.read", sc) {
		return
	}
	list, err := s.Store.ListSharedVariables(ctx, teamFrom(ctx).Team.ID, sc)
	if err != nil {
		s.internal(w, err)
		return
	}
	for _, v := range list {
		if err := s.revealVar(v); err != nil {
			s.internal(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, list)
}

type variableInput struct {
	Scope       rbac.Scope `json:"scope"`
	Key         string     `json:"key"`
	Value       *string    `json:"value"`
	Comment     *string    `json:"comment"`
	IsSecret    *bool      `json:"is_secret"`
	IsMultiline *bool      `json:"is_multiline"`
	IsLiteral   *bool      `json:"is_literal"`
}

// apply fusionne l'entrée dans v ; un secret le reste (sinon sa valeur deviendrait lisible).
func (s *Server) applyVariable(w http.ResponseWriter, in *variableInput, v *store.SharedVariable) bool {
	if in.Value != nil {
		if len(*in.Value) > 64<<10 {
			writeError(w, http.StatusUnprocessableEntity, "valeur trop longue (64 Ko max.)")
			return false
		}
		enc, err := s.Box.Encrypt(*in.Value)
		if err != nil {
			s.internal(w, err)
			return false
		}
		v.ValueEnc = enc
	}
	if in.Comment != nil {
		v.Comment = truncate(strings.TrimSpace(*in.Comment), 500)
	}
	if in.IsSecret != nil {
		if v.IsSecret && !*in.IsSecret && in.Value == nil {
			writeError(w, http.StatusUnprocessableEntity, "un secret ne peut redevenir visible qu'en redéfinissant sa valeur")
			return false
		}
		v.IsSecret = *in.IsSecret
	}
	if in.IsMultiline != nil {
		v.IsMultiline = *in.IsMultiline
	}
	if in.IsLiteral != nil {
		v.IsLiteral = *in.IsLiteral
	}
	return true
}

func (s *Server) createVariable(w http.ResponseWriter, r *http.Request) {
	var in variableInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	team := teamFrom(ctx).Team
	if in.Scope.Type == "" {
		in.Scope = teamFrom(ctx).Scope()
	}
	if !slices.Contains(sharedScopes, in.Scope.Type) {
		writeError(w, http.StatusUnprocessableEntity, "scope.type : team, project, environment ou server")
		return
	}
	if !s.authorize(w, r, "variable.manage", in.Scope) {
		return
	}
	if in.Scope.Type == rbac.ScopeTeam && in.Scope.ID != team.ID {
		writeError(w, http.StatusUnprocessableEntity, "les variables d'une sous-équipe se gèrent depuis cette sous-équipe")
		return
	}
	if !envKeyRe.MatchString(in.Key) {
		writeError(w, http.StatusUnprocessableEntity, "clé invalide (lettres, chiffres et _, sans commencer par un chiffre)")
		return
	}
	if in.Value == nil {
		writeError(w, http.StatusUnprocessableEntity, "valeur requise")
		return
	}
	v := &store.SharedVariable{TeamID: team.ID, Scope: in.Scope, Key: in.Key}
	if !s.applyVariable(w, &in, v) {
		return
	}
	created, err := s.Store.CreateSharedVariable(ctx, v)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "variable.created", "shared_variable", created.ID, map[string]any{"scope": in.Scope, "key": in.Key, "secret": v.IsSecret})
	if err := s.revealVar(created); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) teamVariable(w http.ResponseWriter, r *http.Request) (*store.SharedVariable, bool) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	v, err := s.Store.SharedVariableByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return v, s.authorize(w, r, "variable.manage", v.Scope)
}

func (s *Server) updateVariable(w http.ResponseWriter, r *http.Request) {
	v, ok := s.teamVariable(w, r)
	if !ok {
		return
	}
	var in variableInput
	if !decode(w, r, &in) || !s.applyVariable(w, &in, v) {
		return
	}
	if err := s.Store.UpdateSharedVariable(r.Context(), v); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "variable.updated", "shared_variable", v.ID, map[string]any{"key": v.Key, "value_changed": in.Value != nil})
	if err := s.revealVar(v); err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, v)
}

func (s *Server) deleteVariable(w http.ResponseWriter, r *http.Request) {
	v, ok := s.teamVariable(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteSharedVariable(r.Context(), v.TeamID, v.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "variable.deleted", "shared_variable", v.ID, map[string]any{"scope": v.Scope, "key": v.Key})
	w.WriteHeader(http.StatusNoContent)
}

// --- Tags ---

func (s *Server) listTags(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "team.read", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListTags(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type tagInput struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (in *tagInput) valid(w http.ResponseWriter) bool {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 50 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (50 caractères max.)")
		return false
	}
	if in.Color != "" && !colorRe.MatchString(in.Color) {
		writeError(w, http.StatusUnprocessableEntity, "couleur au format #rrggbb")
		return false
	}
	return true
}

func (s *Server) createTag(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "resource.update", rbac.Scope{}) {
		return
	}
	var in tagInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	t, err := s.Store.CreateTag(r.Context(), teamFrom(r.Context()).Team.ID, in.Name, in.Color)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "tag.created", "tag", t.ID, in)
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) updateTag(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "resource.update", rbac.Scope{}) {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	var in tagInput
	if !decode(w, r, &in) || !in.valid(w) {
		return
	}
	if err := s.Store.UpdateTag(r.Context(), teamFrom(r.Context()).Team.ID, id, in.Name, in.Color); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "tag.updated", "tag", id, in)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteTag(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "resource.update", rbac.Scope{}) {
		return
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return
	}
	if err := s.Store.DeleteTag(r.Context(), teamFrom(r.Context()).Team.ID, id); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "tag.deleted", "tag", id, nil)
	w.WriteHeader(http.StatusNoContent)
}
