package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/Murathan-Aydin/nodra/internal/deploy"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/services"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// --- Catalogue ---

func (s *Server) listServiceTemplates(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	writeJSON(w, http.StatusOK, map[string]any{"templates": services.Catalog(q.Get("q"), q.Get("category")), "categories": services.Categories()})
}

func (s *Server) getServiceTemplate(w http.ResponseWriter, r *http.Request) {
	t, err := services.Get(chi.URLParam(r, "slug"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, t)
}

// --- Préparation : sous-services, domaines et secrets générés ---

// serviceParts déduit les sous-services (domaines générés pour les services exposés) et les variables
// à créer (secrets magiques générés, variables configurables avec leur valeur par défaut).
// existing : variables déjà enregistrées (jamais régénérées).
func (s *Server) serviceParts(svc *store.Service, sv *store.Server, current []*store.ServiceApp, existing map[string]string) ([]*store.ServiceApp, []*store.EnvVar, error) {
	an, err := services.Analyze(svc.DockerComposeRaw)
	if err != nil {
		return nil, nil, err
	}
	have := map[string]*store.ServiceApp{}
	for _, a := range current {
		have[a.Name] = a
	}
	var apps []*store.ServiceApp
	for _, name := range an.Services {
		app := &store.ServiceApp{Name: name, Image: an.Images[name], Domains: []string{}, ExcludeFromStatus: an.Excluded[name]}
		if m, ok := an.Exposed[name]; ok {
			app.URLVar, app.Port = m.ID, m.Port
			if prev := have[name]; prev != nil {
				app.Domains = prev.Domains
			} else if d := defaultDomain(strings.ToLower(strings.ReplaceAll(m.ID, "_", "-")), sv); d != "" {
				if m.Port != "" {
					d += ":" + m.Port // port du conteneur (convention des domaines Nodra)
				}
				app.Domains = []string{d}
			}
		}
		apps = append(apps, app)
	}
	values := map[string]string{}
	for k, v := range existing {
		values[k] = v
	}
	var vars []*store.EnvVar
	add := func(key, value, comment string) error {
		if _, ok := values[key]; ok {
			return nil
		}
		enc, err := s.Box.Encrypt(value)
		if err != nil {
			return err
		}
		values[key] = value
		vars = append(vars, &store.EnvVar{Key: key, ValueEnc: enc, Comment: comment})
		return nil
	}
	for _, m := range an.SortedMagic() {
		if v, ok := services.Generate(m, values); ok {
			if err := add(m.Name, v, "généré par Nodra"); err != nil {
				return nil, nil, err
			}
		}
	}
	for _, u := range an.User {
		comment := ""
		if u.Required {
			comment = "obligatoire"
		}
		if err := add(u.Key, u.Default, comment); err != nil {
			return nil, nil, err
		}
	}
	return apps, vars, nil
}

func (s *Server) existingEnv(ctx context.Context, resourceID string) (map[string]string, error) {
	list, err := s.Store.ListEnvVars(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, v := range list {
		if v.IsPreview {
			continue
		}
		if out[v.Key], err = s.Box.Decrypt(v.ValueEnc); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// --- CRUD ---

func (s *Server) listServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	envID := r.URL.Query().Get("environment_id")
	if envID != "" && !isUUID(envID) {
		writeError(w, http.StatusBadRequest, "environment_id invalide")
		return
	}
	list, err := s.Store.ListServices(ctx, teamFrom(ctx).Team.ID, envID)
	if err != nil {
		s.internal(w, err)
		return
	}
	visible := []*store.Service{}
	for _, svc := range list {
		if ok, err := s.can(ctx, "resource.read", resourceScope(svc.ID)); err != nil {
			s.internal(w, err)
			return
		} else if ok {
			visible = append(visible, svc)
		}
	}
	writeJSON(w, http.StatusOK, visible)
}

type serviceInput struct {
	EnvironmentID    string  `json:"environment_id"`
	DestinationID    string  `json:"destination_id"`
	Template         string  `json:"template"`
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	DockerComposeRaw *string `json:"docker_compose_raw"`
	InstantDeploy    bool    `json:"instant_deploy"`
}

func (s *Server) createService(w http.ResponseWriter, r *http.Request) {
	var in serviceInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	team := teamFrom(ctx).Team
	if !isUUID(in.EnvironmentID) || !isUUID(in.DestinationID) {
		writeError(w, http.StatusUnprocessableEntity, "environment_id et destination_id requis")
		return
	}
	if !s.authorize(w, r, "resource.create", envScope(in.EnvironmentID)) {
		return
	}
	_, sv, err := s.Store.DestinationInTeam(ctx, team.ID, in.DestinationID)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "destination inconnue")
		return
	}
	if !s.authorize(w, r, "server.read", rbac.Scope{Type: rbac.ScopeServer, ID: sv.ID}) {
		return
	}
	svc := &store.Service{TeamID: team.ID, EnvironmentID: in.EnvironmentID, DestinationID: in.DestinationID, Template: in.Template}
	if in.Template != "" {
		t, err := services.Get(in.Template)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		svc.Name, svc.DockerComposeRaw = t.Name, t.Compose
	}
	if in.Name != nil {
		svc.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		svc.Description = strings.TrimSpace(*in.Description)
	}
	if in.DockerComposeRaw != nil {
		svc.DockerComposeRaw = *in.DockerComposeRaw
	}
	if svc.Name == "" || len(svc.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return
	}
	if strings.TrimSpace(svc.DockerComposeRaw) == "" {
		writeError(w, http.StatusUnprocessableEntity, "template ou docker_compose_raw requis")
		return
	}
	apps, vars, err := s.serviceParts(svc, sv, nil, nil)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	created, err := s.Store.CreateService(ctx, svc, apps, vars)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "service.created", "service", created.ID, map[string]string{"name": created.Name, "template": created.Template})
	out := map[string]any{"service": created}
	if in.InstantDeploy {
		uid := userFrom(ctx).ID
		pending, err := s.gate(r, "resource.deploy", resourceScope(created.ID), "Démarrage du service "+created.Name,
			svcActionPayload{created.TeamID, created.ID, deploy.TaskServiceStart, "", &uid})
		if err != nil {
			s.fail(w, err)
			return
		}
		if pending != nil {
			out["approval_request"] = pending
		} else if task, err := s.enqueueServiceTask(ctx, created, deploy.TaskServiceStart, "", &uid); err == nil {
			out["task"] = task
		} else {
			s.internal(w, err)
			return
		}
	}
	s.respondService(w, r, created, out, http.StatusCreated)
}

func (s *Server) respondService(w http.ResponseWriter, r *http.Request, svc *store.Service, out map[string]any, status int) {
	apps, err := s.Store.ServiceApps(r.Context(), svc.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	tags, err := s.Store.ResourceTags(r.Context(), svc.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if out == nil {
		out = map[string]any{}
	}
	out["service"], out["apps"], out["tags"] = svc, apps, tags
	writeJSON(w, status, out)
}

func (s *Server) teamService(w http.ResponseWriter, r *http.Request, perm string) (*store.Service, bool) {
	id, ok := idParam(w, r, "svc")
	if !ok {
		return nil, false
	}
	svc, err := s.Store.ServiceByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return svc, s.authorize(w, r, perm, resourceScope(svc.ID))
}

func (s *Server) svcLoader(w http.ResponseWriter, r *http.Request, perm string) (string, string, string, bool) {
	svc, ok := s.teamService(w, r, perm)
	if !ok {
		return "", "", "", false
	}
	return svc.ID, "service", svc.Name, true
}

func (s *Server) getService(w http.ResponseWriter, r *http.Request) {
	if svc, ok := s.teamService(w, r, "resource.read"); ok {
		s.respondService(w, r, svc, nil, http.StatusOK)
	}
}

func (s *Server) updateService(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.teamService(w, r, "resource.update")
	if !ok {
		return
	}
	var in serviceInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	before := *svc
	if in.Name != nil {
		svc.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		svc.Description = strings.TrimSpace(*in.Description)
	}
	if svc.Name == "" || len(svc.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return
	}
	var apps []*store.ServiceApp
	var vars []*store.EnvVar
	if in.DockerComposeRaw != nil && *in.DockerComposeRaw != svc.DockerComposeRaw {
		svc.DockerComposeRaw = *in.DockerComposeRaw
		loc, err := s.Store.ResourceLocation(ctx, svc.ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		sv, err := s.Store.ServerByIDAny(ctx, loc.ServerID)
		if err != nil {
			s.internal(w, err)
			return
		}
		current, err := s.Store.ServiceApps(ctx, svc.ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		existing, err := s.existingEnv(ctx, svc.ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		if apps, vars, err = s.serviceParts(svc, sv, current, existing); err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	if err := s.Store.UpdateService(ctx, svc, apps, vars); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "service.updated", "service", svc.ID, map[string]any{"name": map[string]string{"before": before.Name, "after": svc.Name},
		"compose_changed": before.DockerComposeRaw != svc.DockerComposeRaw})
	s.respondService(w, r, svc, nil, http.StatusOK)
}

// updateServiceApp : domaines, port et exclusion du statut d'un sous-service.
func (s *Server) updateServiceApp(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.teamService(w, r, "resource.update")
	if !ok {
		return
	}
	name := chi.URLParam(r, "app")
	ctx := r.Context()
	apps, err := s.Store.ServiceApps(ctx, svc.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	var app *store.ServiceApp
	for _, a := range apps {
		if a.Name == name {
			app = a
		}
	}
	if app == nil {
		writeError(w, http.StatusNotFound, "sous-service introuvable")
		return
	}
	var in struct {
		Domains             *[]string `json:"domains"`
		Port                *string   `json:"port"`
		ExcludeFromStatus   *bool     `json:"exclude_from_status"`
		ForceDomainOverride bool      `json:"force_domain_override"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Domains != nil {
		app.Domains = []string{}
		for _, d := range *in.Domains {
			if strings.TrimSpace(d) == "" {
				continue
			}
			n, _, ok := NormalizeAppDomain(d)
			if !ok {
				writeError(w, http.StatusUnprocessableEntity, "domaine invalide : "+d)
				return
			}
			app.Domains = append(app.Domains, n)
		}
		if !in.ForceDomainOverride {
			msg, err := s.checkDomains(ctx, &store.Application{ID: svc.ID, Domains: app.Domains})
			if err != nil {
				s.internal(w, err)
				return
			} else if msg != "" {
				writeError(w, http.StatusConflict, msg)
				return
			}
		}
	}
	if in.Port != nil {
		if *in.Port != "" && !validPorts(*in.Port, false) {
			writeError(w, http.StatusUnprocessableEntity, "port invalide")
			return
		}
		app.Port = *in.Port
	}
	if in.ExcludeFromStatus != nil {
		app.ExcludeFromStatus = *in.ExcludeFromStatus
	}
	if err := s.Store.UpdateServiceApp(ctx, svc.ID, app.Name, app.Domains, app.Port, app.ExcludeFromStatus); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "service.app.updated", "service", svc.ID, map[string]any{"app": app.Name, "domains": app.Domains})
	writeJSON(w, http.StatusOK, app)
}

type svcDeletePayload struct {
	TeamID        string `json:"team_id"`
	ServiceID     string `json:"service_id"`
	DeleteVolumes bool   `json:"delete_volumes"`
}

func (s *Server) deleteService(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.teamService(w, r, "resource.delete")
	if !ok {
		return
	}
	delVolumes := r.URL.Query().Get("delete_volumes") == "true"
	pending, err := s.gate(r, "resource.delete", resourceScope(svc.ID), "Suppression du service "+svc.Name, svcDeletePayload{svc.TeamID, svc.ID, delVolumes})
	if err != nil {
		s.fail(w, err)
		return
	}
	if pending != nil {
		respondPending(w, pending)
		return
	}
	if err := s.removeService(r.Context(), svc, delVolumes); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "service.deleted", "service", svc.ID, map[string]any{"name": svc.Name, "delete_volumes": delVolumes})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeService(ctx context.Context, svc *store.Service, deleteVolumes bool) error {
	if s.Deployer != nil {
		if err := s.Deployer.TeardownService(ctx, svc, deleteVolumes); err != nil {
			return err
		}
	}
	return s.Store.DeleteService(ctx, svc.TeamID, svc.ID)
}

// --- Actions ---

type svcActionPayload struct {
	TeamID    string  `json:"team_id"`
	ServiceID string  `json:"service_id"`
	Kind      string  `json:"kind"`
	App       string  `json:"app,omitempty"`
	CreatedBy *string `json:"created_by,omitempty"`
}

var errServiceBusy = errors.New("une action est déjà en cours sur ce service")

func (s *Server) enqueueServiceTask(ctx context.Context, svc *store.Service, kind, app string, by *string) (*store.ServerTask, error) {
	if busy, err := s.Store.HasActiveServiceTask(ctx, svc.ID); err != nil {
		return nil, err
	} else if busy {
		return nil, errServiceBusy
	}
	loc, err := s.Store.ResourceLocation(ctx, svc.ID)
	if err != nil {
		return nil, err
	}
	params, _ := json.Marshal(deploy.ServiceParams{ServiceID: svc.ID, App: app})
	return s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: svc.TeamID, ServerID: loc.ServerID, Kind: kind, Params: params, CreatedBy: by})
}

func (s *Server) execServiceAction(ctx context.Context, req *store.ApprovalRequest) error {
	var p svcActionPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	svc, err := s.Store.ServiceByID(ctx, p.TeamID, p.ServiceID)
	if err != nil {
		return err
	}
	_, err = s.enqueueServiceTask(ctx, svc, p.Kind, p.App, p.CreatedBy)
	return err
}

// serviceAction : start / restart / restart d'un sous-service (soumis aux approbations) et stop.
func (s *Server) serviceAction(kind string) http.HandlerFunc {
	verbs := map[string]string{deploy.TaskServiceStart: "Démarrage", deploy.TaskServiceRestart: "Redémarrage",
		deploy.TaskServiceStop: "Arrêt", deploy.TaskServiceRestartApp: "Redémarrage d'un conteneur"}
	return func(w http.ResponseWriter, r *http.Request) {
		svc, ok := s.teamService(w, r, "resource.deploy")
		if !ok {
			return
		}
		ctx := r.Context()
		app := chi.URLParam(r, "app")
		if kind == deploy.TaskServiceRestartApp {
			apps, err := s.Store.ServiceApps(ctx, svc.ID)
			if err != nil {
				s.internal(w, err)
				return
			}
			found := false
			for _, a := range apps {
				found = found || a.Name == app
			}
			if !found {
				writeError(w, http.StatusNotFound, "sous-service introuvable")
				return
			}
		}
		loc, err := s.Store.ResourceLocation(ctx, svc.ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		if sv, err := s.Store.ServerByIDAny(ctx, loc.ServerID); err != nil {
			s.internal(w, err)
			return
		} else if !sv.IsUsable {
			writeError(w, http.StatusConflict, "le serveur n'est pas validé")
			return
		}
		uid := userFrom(ctx).ID
		if kind != deploy.TaskServiceStop {
			pending, err := s.gate(r, "resource.deploy", resourceScope(svc.ID), verbs[kind]+" du service "+svc.Name,
				svcActionPayload{svc.TeamID, svc.ID, kind, app, &uid})
			if err != nil {
				s.fail(w, err)
				return
			}
			if pending != nil {
				respondPending(w, pending)
				return
			}
		}
		task, err := s.enqueueServiceTask(ctx, svc, kind, app, &uid)
		if errors.Is(err, errServiceBusy) {
			writeError(w, http.StatusConflict, err.Error())
			return
		} else if err != nil {
			s.internal(w, err)
			return
		}
		s.auditTeam(r, "service."+strings.TrimPrefix(kind, "service_"), "service", svc.ID, map[string]string{"task_id": task.ID, "app": app})
		writeJSON(w, http.StatusAccepted, task)
	}
}

func (s *Server) serviceTasks(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.teamService(w, r, "resource.logs.read")
	if !ok {
		return
	}
	list, err := s.Store.ServiceTasks(r.Context(), svc.ID, 50)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) serviceLogs(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.teamService(w, r, "resource.logs.read")
	if !ok || !s.deployer(w) {
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 || lines > 5000 {
		lines = 100
	}
	app := r.URL.Query().Get("app")
	if app != "" && !serviceRe.MatchString(app) {
		writeError(w, http.StatusBadRequest, "app invalide")
		return
	}
	out, err := s.Deployer.ServiceLogs(r.Context(), svc, app, lines)
	if err != nil {
		writeError(w, http.StatusBadGateway, "serveur inaccessible : "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": out})
}

func (s *Server) setServiceTags(w http.ResponseWriter, r *http.Request) {
	svc, ok := s.teamService(w, r, "resource.update")
	if !ok {
		return
	}
	var in struct {
		TagIDs []string `json:"tag_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	for _, id := range in.TagIDs {
		if !isUUID(id) {
			writeError(w, http.StatusUnprocessableEntity, "tag inconnu")
			return
		}
	}
	if in.TagIDs == nil {
		in.TagIDs = []string{}
	}
	if err := s.Store.SetResourceTags(r.Context(), svc.TeamID, "service", svc.ID, in.TagIDs); err != nil {
		s.internal(w, err)
		return
	}
	tags, err := s.Store.ResourceTags(r.Context(), svc.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "service.tags.updated", "service", svc.ID, in)
	writeJSON(w, http.StatusOK, tags)
}
