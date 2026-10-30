package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/jobs"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// JobQueue enfile les tâches de fond (implémenté par jobs.Jobs).
type JobQueue interface {
	EnqueueServerTask(ctx context.Context, t *store.ServerTask) (*store.ServerTask, error)
	EnqueueDeployment(ctx context.Context, d *store.Deployment, timeout int) (*store.Deployment, error)
	EnqueueBackup(ctx context.Context, b *store.BackupSchedule, by *string) (*store.ServerTask, error)
}

var (
	hostnameRe = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	sshUserRe  = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	domainRe   = regexp.MustCompile(`^https?://([a-zA-Z0-9-]+\.)+[a-zA-Z]{2,}$`)
)

type serverInput struct {
	Name         *string         `json:"name"`
	Description  *string         `json:"description"`
	IP           *string         `json:"ip"`
	Port         *int            `json:"port"`
	User         *string         `json:"user"`
	PrivateKeyID *string         `json:"private_key_id"`
	ProxyType    *string         `json:"proxy_type"` // à la création uniquement (ensuite : PATCH /proxy)
	Settings     json.RawMessage `json:"settings"`
}

// apply fusionne l'entrée dans sv et valide le résultat ; renvoie un message d'erreur.
func (in *serverInput) apply(sv *store.Server) string {
	if in.Name != nil {
		sv.Name = strings.TrimSpace(*in.Name)
	}
	if in.Description != nil {
		sv.Description = strings.TrimSpace(*in.Description)
	}
	if in.IP != nil {
		sv.IP = strings.TrimSpace(*in.IP)
	}
	if in.Port != nil {
		sv.Port = *in.Port
	}
	if in.User != nil {
		sv.User = strings.TrimSpace(*in.User)
	}
	if in.PrivateKeyID != nil {
		sv.PrivateKeyID = *in.PrivateKeyID
	}
	if len(in.Settings) > 0 {
		if err := json.Unmarshal(in.Settings, &sv.Settings); err != nil {
			return "réglages invalides : " + err.Error()
		}
	}
	switch {
	case sv.Name == "" || len(sv.Name) > 100:
		return "nom requis (100 caractères max.)"
	case net.ParseIP(sv.IP) == nil && !hostnameRe.MatchString(sv.IP):
		return "adresse IP ou nom d'hôte invalide"
	case sv.Port < 1 || sv.Port > 65535:
		return "port invalide"
	case !sshUserRe.MatchString(sv.User):
		return "utilisateur SSH invalide"
	case !isUUID(sv.PrivateKeyID):
		return "private_key_id requis"
	}
	return validSettings(sv.Settings)
}

func validSettings(st store.ServerSettings) string {
	if _, err := jobs.ParseCron(st.DockerCleanupFrequency); err != nil {
		return "docker_cleanup_frequency : expression cron invalide"
	}
	if _, err := time.LoadLocation(st.Timezone); err != nil || st.Timezone == "" {
		return "timezone invalide"
	}
	switch {
	case st.DockerCleanupThreshold < 0 || st.DockerCleanupThreshold > 100:
		return "docker_cleanup_threshold doit être entre 0 et 100"
	case st.ConcurrentBuilds < 1 || st.ConcurrentBuilds > 20:
		return "concurrent_builds doit être entre 1 et 20"
	case st.DeploymentTimeout < 60 || st.DeploymentTimeout > 86400:
		return "deployment_timeout doit être entre 60 et 86400 secondes"
	case st.MetricsRetentionDays < 1 || st.MetricsRetentionDays > 90:
		return "metrics_retention_days doit être entre 1 et 90"
	case st.ConnectionTimeout < 1 || st.ConnectionTimeout > 120:
		return "connection_timeout doit être entre 1 et 120 secondes"
	case st.WildcardDomain != "" && !domainRe.MatchString(st.WildcardDomain):
		return "wildcard_domain doit être de la forme https://exemple.com"
	}
	return ""
}

func (s *Server) listServers(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "server.read", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListServers(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createServer(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "server.create", rbac.Scope{}) {
		return
	}
	var in serverInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	team := teamFrom(ctx).Team
	sv := &store.Server{TeamID: team.ID, Port: 22, User: "root", Settings: store.DefaultServerSettings(),
		Proxy: store.ProxyInfo{Type: servers.ProxyTraefik}}
	if in.ProxyType != nil {
		if !slices.Contains([]string{servers.ProxyTraefik, servers.ProxyCaddy, servers.ProxyNone}, *in.ProxyType) {
			writeError(w, http.StatusUnprocessableEntity, "proxy_type : traefik, caddy ou none")
			return
		}
		sv.Proxy.Type = *in.ProxyType
	}
	if msg := in.apply(sv); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if _, err := s.Store.PrivateKeyByID(ctx, team.ID, sv.PrivateKeyID); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "clé privée inconnue")
		return
	}
	created, err := s.Store.CreateServer(ctx, sv)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "server.created", "server", created.ID, map[string]any{"name": created.Name, "ip": created.IP})
	// Validation lancée automatiquement.
	task, err := s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: team.ID, ServerID: created.ID,
		Kind: servers.TaskValidate, CreatedBy: &userFrom(ctx).ID})
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"server": created, "task": task})
}

// teamServer charge le serveur {server} de l'équipe et vérifie la permission sur sa portée.
func (s *Server) teamServer(w http.ResponseWriter, r *http.Request, perm string) (*store.Server, bool) {
	id, ok := idParam(w, r, "server")
	if !ok {
		return nil, false
	}
	sv, err := s.Store.ServerByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return sv, s.authorize(w, r, perm, rbac.Scope{Type: rbac.ScopeServer, ID: sv.ID})
}

func (s *Server) getServer(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	dests, err := s.Store.ListDestinations(r.Context(), sv.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"server": sv, "destinations": dests})
}

func (s *Server) updateServer(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.update")
	if !ok {
		return
	}
	var in serverInput
	if !decode(w, r, &in) {
		return
	}
	before := *sv
	if msg := in.apply(sv); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if sv.PrivateKeyID != before.PrivateKeyID {
		if _, err := s.Store.PrivateKeyByID(r.Context(), sv.TeamID, sv.PrivateKeyID); err != nil {
			writeError(w, http.StatusUnprocessableEntity, "clé privée inconnue")
			return
		}
	}
	if err := s.Store.UpdateServer(r.Context(), sv); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "server.updated", "server", sv.ID, map[string]any{"before": before, "after": sv})
	updated, err := s.Store.ServerByID(r.Context(), sv.TeamID, sv.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

type deleteServerPayload struct {
	TeamID   string `json:"team_id"`
	ServerID string `json:"server_id"`
}

func (s *Server) deleteServer(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.delete")
	if !ok {
		return
	}
	scope := rbac.Scope{Type: rbac.ScopeServer, ID: sv.ID}
	req, err := s.gate(r, "server.delete", scope, "Suppression du serveur "+sv.Name, deleteServerPayload{sv.TeamID, sv.ID})
	if err != nil {
		s.fail(w, err)
		return
	}
	if req != nil {
		respondPending(w, req)
		return
	}
	if err := s.Store.DeleteServer(r.Context(), sv.TeamID, sv.ID); err != nil {
		if errors.Is(err, store.ErrInUse) {
			writeError(w, http.StatusConflict, "des ressources sont encore déployées sur ce serveur")
			return
		}
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "server.deleted", "server", sv.ID, map[string]any{"name": sv.Name, "ip": sv.IP})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) execDeleteServer(ctx context.Context, req *store.ApprovalRequest) error {
	var p deleteServerPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	if err := s.Store.DeleteServer(ctx, p.TeamID, p.ServerID); errors.Is(err, store.ErrInUse) {
		return errors.New("des ressources sont encore déployées sur ce serveur")
	} else if err != nil {
		return err
	}
	return nil
}

// --- Tâches ---

// startTask enfile une tâche sur le serveur et répond 202.
func (s *Server) startTask(w http.ResponseWriter, r *http.Request, sv *store.Server, kind string, params any, auditMeta any) {
	ctx := r.Context()
	var raw json.RawMessage
	if params != nil {
		raw, _ = json.Marshal(params)
	}
	task, err := s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: sv.TeamID, ServerID: sv.ID, Kind: kind,
		Params: raw, CreatedBy: &userFrom(ctx).ID})
	if err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "server.task."+kind, "server", sv.ID, auditMeta)
	writeJSON(w, http.StatusAccepted, task)
}

// serverAction : route POST simple qui lance une tâche sans paramètre.
func (s *Server) serverAction(kind, perm string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sv, ok := s.teamServer(w, r, perm); ok {
			s.startTask(w, r, sv, kind, nil, nil)
		}
	}
}

func (s *Server) runCommand(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.terminal")
	if !ok {
		return
	}
	var in servers.CommandParams
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.Command) == "" || len(in.Command) > 10000 {
		writeError(w, http.StatusUnprocessableEntity, "commande requise (10 000 caractères max.)")
		return
	}
	s.startTask(w, r, sv, servers.TaskCommand, in, map[string]string{"command": in.Command})
}

func (s *Server) startCloudflared(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.update")
	if !ok {
		return
	}
	var in struct {
		Token string `json:"token"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Token = strings.TrimSpace(in.Token)
	if len(in.Token) < 20 {
		writeError(w, http.StatusUnprocessableEntity, "jeton Cloudflare Tunnel requis")
		return
	}
	enc, err := s.Box.Encrypt(in.Token)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.startTask(w, r, sv, servers.TaskCloudflaredStart, servers.CloudflaredParams{TokenEnc: enc}, nil)
}

func (s *Server) resetHostKey(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.update")
	if !ok {
		return
	}
	if err := s.Store.ResetServerHostKey(r.Context(), sv.TeamID, sv.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "server.host_key.reset", "server", sv.ID, map[string]string{"previous": sv.HostKey})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listServerTasks(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	list, err := s.Store.ListServerTasks(r.Context(), sv.ID, 50)
	if err != nil {
		s.internal(w, err)
		return
	}
	// La liste n'inclut pas la sortie complète (voir GET /tasks/{task}).
	for _, t := range list {
		t.Output = ""
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) serverTask(w http.ResponseWriter, r *http.Request) (*store.ServerTask, bool) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return nil, false
	}
	id, ok := idParam(w, r, "task")
	if !ok {
		return nil, false
	}
	t, err := s.Store.ServerTaskByID(r.Context(), id)
	if err != nil || t.ServerID != sv.ID {
		writeError(w, http.StatusNotFound, "introuvable")
		return nil, false
	}
	return t, true
}

func (s *Server) getServerTask(w http.ResponseWriter, r *http.Request) {
	if t, ok := s.serverTask(w, r); ok {
		writeJSON(w, http.StatusOK, t)
	}
}

func (s *Server) cancelServerTask(w http.ResponseWriter, r *http.Request) {
	t, ok := s.serverTask(w, r)
	if !ok || !s.authorize(w, r, "server.update", rbac.Scope{Type: rbac.ScopeServer, ID: t.ServerID}) {
		return
	}
	done, err := s.Store.CancelServerTask(r.Context(), t.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if !done {
		writeError(w, http.StatusConflict, "seule une tâche encore en file peut être annulée")
		return
	}
	s.auditTeam(r, "server.task.cancelled", "server_task", t.ID, nil)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) serverMetrics(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	since := time.Now().Add(-time.Hour)
	if v := r.URL.Query().Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since doit être une date RFC 3339")
			return
		}
		since = t
	}
	list, err := s.Store.ServerMetrics(r.Context(), sv.ID, since)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// --- Destinations (réseaux Docker) ---

func (s *Server) listDestinations(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	list, err := s.Store.ListDestinations(r.Context(), sv.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createDestination(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.update")
	if !ok {
		return
	}
	var in servers.NetworkParams
	if !decode(w, r, &in) {
		return
	}
	in.Name, in.DestinationID = strings.TrimSpace(in.Name), ""
	if in.Name == "" {
		in.Name = in.Network
	}
	if !servers.NetworkRe.MatchString(in.Network) {
		writeError(w, http.StatusUnprocessableEntity, "nom de réseau Docker invalide")
		return
	}
	if !sv.IsUsable {
		writeError(w, http.StatusConflict, "le serveur doit d'abord être validé")
		return
	}
	s.startTask(w, r, sv, servers.TaskNetworkCreate, in, in)
}

func (s *Server) deleteDestination(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.update")
	if !ok {
		return
	}
	id, ok := idParam(w, r, "dest")
	if !ok {
		return
	}
	d, err := s.Store.DestinationByID(r.Context(), sv.ID, id)
	if err != nil {
		s.fail(w, err)
		return
	}
	if d.IsDefault {
		writeError(w, http.StatusConflict, "la destination par défaut ne peut pas être supprimée")
		return
	}
	p := servers.NetworkParams{Name: d.Name, Network: d.Network, DestinationID: d.ID}
	s.startTask(w, r, sv, servers.TaskNetworkDelete, p, p)
}
