package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

type cloneInput struct {
	Name          string `json:"name"`
	EnvironmentID string `json:"environment_id"`
	DestinationID string `json:"destination_id"`
}

// cloneTarget lit et vérifie la cible d'une copie (environnement et destination de l'équipe, droit de
// création) ; par défaut, même environnement et même destination que l'original.
func (s *Server) cloneTarget(w http.ResponseWriter, r *http.Request, name, envID, destID string) (*cloneInput, *store.Server, bool) {
	in := &cloneInput{}
	if r.ContentLength > 0 && !decode(w, r, in) {
		return nil, nil, false
	}
	if in.Name = strings.TrimSpace(in.Name); in.Name == "" {
		in.Name = name + " (copie)"
	}
	if len(in.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom : 100 caractères max.")
		return nil, nil, false
	}
	if in.EnvironmentID == "" {
		in.EnvironmentID = envID
	}
	if in.DestinationID == "" {
		in.DestinationID = destID
	}
	if !isUUID(in.EnvironmentID) || !isUUID(in.DestinationID) {
		writeError(w, http.StatusUnprocessableEntity, "environment_id et destination_id invalides")
		return nil, nil, false
	}
	if !s.authorize(w, r, "resource.create", envScope(in.EnvironmentID)) {
		return nil, nil, false
	}
	ctx := r.Context()
	_, sv, err := s.Store.DestinationInTeam(ctx, teamFrom(ctx).Team.ID, in.DestinationID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "destination inconnue")
		return nil, nil, false
	}
	return in, sv, true
}

func (s *Server) cloned(w http.ResponseWriter, r *http.Request, kind, src, id string, err error) {
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusUnprocessableEntity, "environnement inconnu")
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, kind+".cloned", kind, id, map[string]string{"source": src})
	writeJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Server) cloneApplication(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.read")
	if !ok {
		return
	}
	in, sv, ok := s.cloneTarget(w, r, a.Name, a.EnvironmentID, a.DestinationID)
	if !ok {
		return
	}
	id, err := s.cloneApp(r.Context(), a, in.Name, in.EnvironmentID, in.DestinationID, sv)
	s.cloned(w, r, "application", a.ID, id, err)
}

func (s *Server) cloneDatabase(w http.ResponseWriter, r *http.Request) {
	d, ok := s.teamDatabase(w, r, "resource.read")
	if !ok {
		return
	}
	in, _, ok := s.cloneTarget(w, r, d.Name, d.EnvironmentID, d.DestinationID)
	if !ok {
		return
	}
	id, err := s.cloneDB(r.Context(), d, in.Name, in.EnvironmentID, in.DestinationID)
	s.cloned(w, r, "database", d.ID, id, err)
}

func (s *Server) cloneService(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.teamService(w, r, "resource.read")
	if !ok {
		return
	}
	in, sv, ok := s.cloneTarget(w, r, svc.Name, svc.EnvironmentID, svc.DestinationID)
	if !ok {
		return
	}
	id, err := s.cloneSvc(r.Context(), svc, in.Name, in.EnvironmentID, in.DestinationID, sv)
	s.cloned(w, r, "service", svc.ID, id, err)
}

// cloneApp : copie d'une application (jamais les domaines de l'original : domaine généré s'il en avait).
func (s *Server) cloneApp(ctx context.Context, a *store.Application, name, envID, destID string, sv *store.Server) (string, error) {
	secret, err := s.Box.Encrypt(randomHex(32))
	if err != nil {
		return "", err
	}
	domains := []string{}
	if len(a.Domains) > 0 && a.BuildPack != "dockercompose" {
		if d := defaultDomain(name, sv); d != "" {
			domains = append(domains, d)
		}
	}
	return s.Store.CloneResource(ctx, "applications", a.ID, map[string]any{"name": name, "environment_id": envID,
		"destination_id": destID, "status": "stopped", "domains": domains, "docker_compose_domains": map[string]any{},
		"webhook_secret_enc": secret, "config_hash": ""}, nil)
}

// cloneDB : copie d'une base, sans exposition publique (port pris par l'original) ni données (nouveaux volumes).
func (s *Server) cloneDB(ctx context.Context, d *store.Database, name, envID, destID string) (string, error) {
	return s.Store.CloneResource(ctx, "databases", d.ID, map[string]any{"name": name, "environment_id": envID,
		"destination_id": destID, "status": "stopped", "is_public": false, "public_port": nil, "started_at": nil}, nil)
}

// cloneSvc : copie d'un service ; domaines régénérés pour les sous-services qui en avaient.
func (s *Server) cloneSvc(ctx context.Context, svc *store.Service, name, envID, destID string, sv *store.Server) (string, error) {
	apps, err := s.Store.ServiceApps(ctx, svc.ID)
	if err != nil {
		return "", err
	}
	id, err := s.Store.CloneResource(ctx, "services", svc.ID, map[string]any{"name": name, "environment_id": envID,
		"destination_id": destID, "status": "stopped"}, map[string]any{"domains": []string{}})
	if err != nil {
		return "", err
	}
	for _, app := range apps {
		if len(app.Domains) == 0 {
			continue
		}
		label := app.URLVar
		if label == "" {
			label = app.Name
		}
		if d := defaultDomain(strings.ToLower(strings.ReplaceAll(label, "_", "-")), sv); d != "" {
			if app.Port != "" {
				d += ":" + app.Port // port du conteneur (convention des domaines Nodra)
			}
			if err := s.Store.UpdateServiceApp(ctx, id, app.Name, []string{d}, app.Port, app.ExcludeFromStatus); err != nil {
				return "", err
			}
		}
	}
	return id, nil
}

// cloneEnvResources copie dans un environnement les ressources lisibles d'un autre (mêmes destinations).
func (s *Server) cloneEnvResources(ctx context.Context, teamID, srcEnv, dstEnv string) (int, error) {
	servers := map[string]*store.Server{}
	server := func(destID string) (*store.Server, error) {
		if sv, ok := servers[destID]; ok {
			return sv, nil
		}
		_, sv, err := s.Store.DestinationInTeam(ctx, teamID, destID)
		servers[destID] = sv
		return sv, err
	}
	readable := func(id string) bool { ok, _ := s.can(ctx, "resource.read", resourceScope(id)); return ok }
	n := 0
	apps, err := s.Store.ListApplications(ctx, teamID, srcEnv)
	if err != nil {
		return n, err
	}
	for _, a := range apps {
		if !readable(a.ID) {
			continue
		}
		sv, err := server(a.DestinationID)
		if err != nil {
			return n, err
		}
		if _, err := s.cloneApp(ctx, a, a.Name, dstEnv, a.DestinationID, sv); err != nil {
			return n, err
		}
		n++
	}
	dbs, err := s.Store.ListDatabases(ctx, teamID, srcEnv)
	if err != nil {
		return n, err
	}
	for _, d := range dbs {
		if !readable(d.ID) {
			continue
		}
		if _, err := s.cloneDB(ctx, d, d.Name, dstEnv, d.DestinationID); err != nil {
			return n, err
		}
		n++
	}
	svcs, err := s.Store.ListServices(ctx, teamID, srcEnv)
	if err != nil {
		return n, err
	}
	for _, svc := range svcs {
		if !readable(svc.ID) {
			continue
		}
		sv, err := server(svc.DestinationID)
		if err != nil {
			return n, err
		}
		if _, err := s.cloneSvc(ctx, svc, svc.Name, dstEnv, svc.DestinationID, sv); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
