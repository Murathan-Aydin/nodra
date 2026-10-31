package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Murathan-Aydin/nodra/internal/store"
	"github.com/Murathan-Aydin/nodra/internal/version"
)

func (s *Server) getVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"version": version.Version})
}

// visibleResources : ressources de l'équipe filtrées par le droit resource.read.
func (s *Server) visibleResources(ctx context.Context, f store.ResourceFilter) ([]*store.ResourceSummary, error) {
	list, err := s.Store.TeamResources(ctx, teamFrom(ctx).Team.ID, f)
	if err != nil {
		return nil, err
	}
	out := []*store.ResourceSummary{}
	for _, res := range list {
		ok, err := s.can(ctx, "resource.read", resourceScope(res.ID))
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, res)
		}
	}
	return out, nil
}

// listResources : liste unifiée des applications, bases et services (filtres type, environnement, serveur, tag).
func (s *Server) listResources(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.ResourceFilter{Type: q.Get("type"), EnvironmentID: q.Get("environment_id"), ServerID: q.Get("server_id"), Tag: q.Get("tag")}
	if !slices.Contains([]string{"", "application", "database", "service"}, f.Type) {
		writeError(w, http.StatusBadRequest, "type : application, database ou service")
		return
	}
	if (f.EnvironmentID != "" && !isUUID(f.EnvironmentID)) || (f.ServerID != "" && !isUUID(f.ServerID)) {
		writeError(w, http.StatusBadRequest, "identifiant invalide")
		return
	}
	list, err := s.visibleResources(r.Context(), f)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) serverResources(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	list, err := s.visibleResources(r.Context(), store.ResourceFilter{ServerID: sv.ID})
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// serverDomains : domaines servis par le serveur, avec la ressource qui les porte.
func (s *Server) serverDomains(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	list, err := s.visibleResources(r.Context(), store.ResourceFilter{ServerID: sv.ID})
	if err != nil {
		s.internal(w, err)
		return
	}
	out := []map[string]string{}
	for _, res := range list {
		for _, d := range res.Domains {
			out = append(out, map[string]string{"domain": d, "resource_id": res.ID, "type": res.Type, "name": res.Name})
		}
	}
	slices.SortFunc(out, func(a, b map[string]string) int { return strings.Compare(a["domain"], b["domain"]) })
	writeJSON(w, http.StatusOK, out)
}

// listTeamDeployments : déploiements récents de l'équipe, filtrables par statut.
func (s *Server) listTeamDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := r.URL.Query().Get("status")
	if !slices.Contains([]string{"", "queued", "in_progress", "finished", "failed", "cancelled"}, status) {
		writeError(w, http.StatusBadRequest, "status : queued, in_progress, finished, failed ou cancelled")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	list, err := s.Store.TeamDeployments(ctx, teamFrom(ctx).Team.ID, status, limit)
	if err != nil {
		s.internal(w, err)
		return
	}
	out := []*store.Deployment{}
	for _, d := range list {
		if ok, err := s.can(ctx, "resource.read", resourceScope(d.ApplicationID)); err != nil {
			s.internal(w, err)
			return
		} else if ok {
			out = append(out, d)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// bulkDeploy déploie (applications) ou démarre (bases, services) plusieurs ressources désignées par
// identifiant ou par tag. Chaque ressource passe par la route unitaire correspondante, pour le même
// appelant : mêmes droits, scopes de jeton, approbations et journal d'audit.
func (s *Server) bulkDeploy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Resources    []string `json:"resources"`
		Tags         []string `json:"tags"`
		ForceRebuild bool     `json:"force_rebuild"`
	}
	if r.ContentLength > 0 && !decode(w, r, &in) {
		return
	}
	q := r.URL.Query()
	in.Resources = append(in.Resources, splitList(q.Get("uuid"))...)
	in.Tags = append(in.Tags, splitList(q.Get("tag"))...)
	if q.Get("force") == "true" {
		in.ForceRebuild = true
	}
	ctx := r.Context()
	team := teamFrom(ctx).Team.ID
	ids := []string{}
	for _, id := range in.Resources {
		if !isUUID(id) {
			writeError(w, http.StatusUnprocessableEntity, "identifiant invalide : "+id)
			return
		}
		ids = append(ids, id)
	}
	if len(in.Tags) > 0 {
		tagged, err := s.Store.ResourcesByTags(ctx, team, in.Tags)
		if err != nil {
			s.internal(w, err)
			return
		}
		ids = append(ids, tagged...)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	switch {
	case len(ids) == 0:
		writeError(w, http.StatusUnprocessableEntity, "aucune ressource : resources/uuid ou tags/tag requis")
		return
	case len(ids) > 100:
		writeError(w, http.StatusUnprocessableEntity, "100 ressources au maximum")
		return
	}
	body, _ := json.Marshal(map[string]bool{"force_rebuild": in.ForceRebuild})
	results := []map[string]any{}
	for _, id := range ids {
		typ, err := s.Store.TeamResourceType(ctx, team, id)
		if err != nil {
			results = append(results, map[string]any{"resource_id": id, "status": http.StatusNotFound, "response": map[string]string{"error": "introuvable"}})
			continue
		}
		path := map[string]string{"application": "/applications/%s/deploy", "database": "/databases/%s/start", "service": "/services/%s/start"}[typ]
		rec := s.subRequest(r, http.MethodPost, "/api/v1/teams/"+team+fmt.Sprintf(path, id), body)
		var resp any
		_ = json.Unmarshal(rec.Body.Bytes(), &resp)
		results = append(results, map[string]any{"resource_id": id, "type": typ, "status": rec.Code, "response": resp})
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// subRequest exécute une route de l'API pour le même appelant (cookies ou jeton repris) et renvoie la réponse.
func (s *Server) subRequest(r *http.Request, method, path string, body []byte) *httptest.ResponseRecorder {
	// Contexte de routage chi remis à zéro : sinon le routeur croit traiter une sous-route.
	ctx := context.WithValue(r.Context(), chi.RouteCtxKey, (*chi.Context)(nil))
	req, _ := http.NewRequestWithContext(ctx, method, path, bytes.NewReader(body))
	req.Header = r.Header.Clone()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Requested-With", "nodra")
	req.RemoteAddr = r.RemoteAddr
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec
}

// --- Déplacement vers un autre environnement ---

func (s *Server) moveResource(w http.ResponseWriter, r *http.Request, id, table, kind string) {
	var in struct {
		EnvironmentID string `json:"environment_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !isUUID(in.EnvironmentID) {
		writeError(w, http.StatusUnprocessableEntity, "environment_id requis")
		return
	}
	if !s.authorize(w, r, "resource.create", envScope(in.EnvironmentID)) {
		return
	}
	ctx := r.Context()
	if err := s.Store.MoveResource(ctx, table, teamFrom(ctx).Team.ID, id, in.EnvironmentID); err != nil {
		if err == store.ErrNotFound {
			writeError(w, http.StatusNotFound, "environnement introuvable dans l'équipe")
			return
		}
		s.internal(w, err)
		return
	}
	s.auditTeam(r, kind+".moved", kind, id, map[string]string{"environment_id": in.EnvironmentID})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) moveApplication(w http.ResponseWriter, r *http.Request) {
	if a, ok := s.teamApp(w, r, "resource.update"); ok {
		s.moveResource(w, r, a.ID, "applications", "application")
	}
}

func (s *Server) moveDatabase(w http.ResponseWriter, r *http.Request) {
	if d, ok := s.teamDatabase(w, r, "resource.update"); ok {
		s.moveResource(w, r, d.ID, "databases", "database")
	}
}

func (s *Server) moveService(w http.ResponseWriter, r *http.Request) {
	if svc, ok := s.teamService(w, r, "resource.update"); ok {
		s.moveResource(w, r, svc.ID, "services", "service")
	}
}
