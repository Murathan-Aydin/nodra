package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// registryHostRe : hôte de registre (avec port facultatif), sans schéma ni chemin.
var registryHostRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:[0-9]{1,5})?$`)

type registryInput struct {
	Name     *string `json:"name"`
	Host     *string `json:"host"`
	Username *string `json:"username"`
	Password *string `json:"password"`
}

func (s *Server) applyRegistry(in *registryInput, g *store.Registry) string {
	if in.Name != nil {
		g.Name = strings.TrimSpace(*in.Name)
	}
	if in.Host != nil {
		h := strings.ToLower(strings.TrimSpace(*in.Host))
		h = strings.TrimPrefix(strings.TrimPrefix(h, "https://"), "http://")
		g.Host = strings.TrimSuffix(h, "/")
	}
	if in.Username != nil {
		g.Username = strings.TrimSpace(*in.Username)
	}
	if in.Password != nil && *in.Password != "" && *in.Password != "********" {
		enc, err := s.Box.Encrypt(*in.Password)
		if err != nil {
			return "chiffrement impossible"
		}
		g.PasswordEnc = enc
	}
	switch {
	case g.Name == "" || len(g.Name) > 100:
		return "nom requis (100 caractères max.)"
	case !registryHostRe.MatchString(g.Host):
		return "host : hôte du registre, ex. ghcr.io, registry.gitlab.com ou registry.example.com:5000 (docker.io pour Docker Hub)"
	case g.Username == "" || strings.ContainsAny(g.Username, ":\r\n"):
		return "username requis"
	case g.PasswordEnc == "":
		return "password requis (jeton d'accès de préférence)"
	}
	return ""
}

func (s *Server) listRegistries(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "registry.read", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListRegistries(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createRegistry(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "registry.manage", rbac.Scope{}) {
		return
	}
	var in registryInput
	if !decode(w, r, &in) {
		return
	}
	g := &store.Registry{TeamID: teamFrom(r.Context()).Team.ID}
	if msg := s.applyRegistry(&in, g); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	created, err := s.Store.CreateRegistry(r.Context(), g)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "registry.created", "registry", created.ID, map[string]string{"host": g.Host, "username": g.Username})
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) teamRegistry(w http.ResponseWriter, r *http.Request) (*store.Registry, bool) {
	if !s.authorize(w, r, "registry.manage", rbac.Scope{}) {
		return nil, false
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	g, err := s.Store.RegistryByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return g, true
}

func (s *Server) updateRegistry(w http.ResponseWriter, r *http.Request) {
	g, ok := s.teamRegistry(w, r)
	if !ok {
		return
	}
	var in registryInput
	if !decode(w, r, &in) {
		return
	}
	if msg := s.applyRegistry(&in, g); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if err := s.Store.UpdateRegistry(r.Context(), g); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "registry.updated", "registry", g.ID, map[string]string{"host": g.Host, "username": g.Username})
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) deleteRegistry(w http.ResponseWriter, r *http.Request) {
	g, ok := s.teamRegistry(w, r)
	if !ok {
		return
	}
	if err := s.Store.DeleteRegistry(r.Context(), g.TeamID, g.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "registry.deleted", "registry", g.ID, map[string]string{"host": g.Host})
	w.WriteHeader(http.StatusNoContent)
}
