package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/deploy"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

var (
	dbUserRe       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,62}$`)
	dbNameRe       = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,62}$`)
	initScriptRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,99}$`)
	authMethodRe   = regexp.MustCompile(`^[a-z0-9-]{0,30}$`)
	reservedDBPort = map[int]bool{80: true, 443: true}
)

type dbInput struct {
	EnvironmentID          string              `json:"environment_id"`
	DestinationID          string              `json:"destination_id"`
	Type                   string              `json:"type"`
	Name                   *string             `json:"name"`
	Description            *string             `json:"description"`
	Image                  *string             `json:"image"`
	Username               *string             `json:"username"`
	Password               *string             `json:"password"`
	RootPassword           *string             `json:"root_password"`
	DatabaseName           *string             `json:"database_name"`
	CustomConfig           *string             `json:"custom_config"`
	InitScripts            *[]store.InitScript `json:"init_scripts"`
	IsPublic               *bool               `json:"is_public"`
	PublicPort             *int                `json:"public_port"`
	PortsMappings          *string             `json:"ports_mappings"`
	CustomDockerRunOptions *string             `json:"custom_docker_run_options"`
	Limits                 json.RawMessage     `json:"limits"`
	Settings               json.RawMessage     `json:"settings"`
}

// randomPassword : mot de passe alphanumérique (aucun caractère à échapper dans une URL ou un shell).
func randomPassword(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		k, _ := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		b[i] = alphabet[k.Int64()]
	}
	return string(b)
}

func validSecret(p string) bool {
	return len(p) >= 8 && len(p) <= 256 && !strings.ContainsAny(p, "\x00\n\r")
}

// apply fusionne l'entrée (mots de passe chiffrés) et valide les champs indépendants de la base.
func (in *dbInput) apply(s *Server, d *store.Database) string {
	t := deploy.DBTypes[d.Type]
	for _, f := range []struct {
		dst *string
		v   *string
	}{{&d.Name, in.Name}, {&d.Description, in.Description}, {&d.Image, in.Image}, {&d.Username, in.Username},
		{&d.DatabaseName, in.DatabaseName}, {&d.PortsMappings, in.PortsMappings}, {&d.CustomDockerRunOptions, in.CustomDockerRunOptions}} {
		if f.v != nil {
			*f.dst = strings.TrimSpace(*f.v)
		}
	}
	if in.CustomConfig != nil {
		d.CustomConfig = *in.CustomConfig
	}
	if in.InitScripts != nil {
		d.InitScripts = *in.InitScripts
	}
	if in.IsPublic != nil {
		d.IsPublic = *in.IsPublic
	}
	if in.PublicPort != nil {
		d.PublicPort = in.PublicPort
		if *in.PublicPort == 0 {
			d.PublicPort = nil
		}
	}
	for _, x := range []struct {
		raw json.RawMessage
		dst any
		nm  string
	}{{in.Limits, &d.Limits, "limits"}, {in.Settings, &d.Settings, "settings"}} {
		if len(x.raw) > 0 {
			if err := json.Unmarshal(x.raw, x.dst); err != nil {
				return x.nm + " invalide : " + err.Error()
			}
		}
	}
	for _, p := range []struct {
		v   *string
		dst *string
		ok  bool
		nm  string
	}{{in.Password, &d.PasswordEnc, true, "password"}, {in.RootPassword, &d.RootPasswordEnc, t.Root, "root_password"}} {
		if p.v == nil {
			continue
		}
		if !p.ok {
			return p.nm + " : non pris en charge pour " + d.Type
		}
		if !validSecret(*p.v) {
			return p.nm + " : 8 à 256 caractères, sur une ligne"
		}
		enc, err := s.Box.Encrypt(*p.v)
		if err != nil {
			return "chiffrement impossible"
		}
		*p.dst = enc
	}
	return validateDatabase(d)
}

func validateDatabase(d *store.Database) string {
	t := deploy.DBTypes[d.Type]
	st := d.Settings
	switch {
	case d.Name == "" || len(d.Name) > 100:
		return "nom requis (100 caractères max.)"
	case !imageRe.MatchString(d.Image):
		return "image invalide"
	case t.User != "" && !dbUserRe.MatchString(d.Username), t.User == "" && d.Username != "":
		return "username invalide (non configurable pour " + d.Type + ")"
	case (d.Type == "mysql" || d.Type == "mariadb") && strings.EqualFold(d.Username, "root"):
		return "username : root est réservé (utilisez root_password)"
	case t.DB != "" && !dbNameRe.MatchString(d.DatabaseName), t.DB == "" && d.DatabaseName != "":
		return "database_name invalide (non configurable pour " + d.Type + ")"
	case d.CustomConfig != "" && t.ConfigPath == "":
		return "custom_config : non pris en charge pour " + d.Type
	case len(d.CustomConfig) > 64<<10:
		return "custom_config trop long (64 Ko max.)"
	case len(d.InitScripts) > 0 && t.InitDir == "":
		return "init_scripts : non pris en charge pour " + d.Type
	case d.PublicPort != nil && (*d.PublicPort < 1 || *d.PublicPort > 65535 || reservedDBPort[*d.PublicPort]):
		return "public_port : 1 à 65535, hors 80 et 443 (proxy)"
	case d.IsPublic && d.PublicPort == nil:
		return "public_port requis pour exposer la base"
	case !validPorts(d.PortsMappings, true):
		return "ports_mappings : liste hôte:conteneur (ex. 5433:5432)"
	case !validLimits(d.Limits):
		return limitsError
	case st.StopGracePeriod < 0 || st.StopGracePeriod > 3600:
		return "settings.stop_grace_period : 0 à 3600"
	case !authMethodRe.MatchString(st.PostgresHostAuthMethod),
		len(st.PostgresInitdbArgs) > 500 || strings.ContainsAny(st.PostgresInitdbArgs, "\x00\n\r"):
		return "settings : options PostgreSQL invalides"
	case (st.PostgresHostAuthMethod != "" || st.PostgresInitdbArgs != "") && d.Type != "postgresql":
		return "settings : options réservées à PostgreSQL"
	}
	seen, size := map[string]bool{}, 0
	for _, sc := range d.InitScripts {
		if !initScriptRe.MatchString(sc.Filename) || seen[sc.Filename] {
			return "init_scripts : nom de fichier invalide ou en double " + sc.Filename
		}
		seen[sc.Filename] = true
		size += len(sc.Content)
	}
	if size > 1<<20 {
		return "init_scripts trop longs (1 Mo max. au total)"
	}
	return ""
}

// checkHostPorts signale un port public ou un port publié déjà utilisé sur le serveur.
func (s *Server) checkHostPorts(ctx context.Context, d *store.Database, sv *store.Server) (string, error) {
	var wanted []int
	if d.IsPublic && d.PublicPort != nil {
		wanted = append(wanted, *d.PublicPort)
	}
	for _, m := range strings.Split(d.PortsMappings, ",") {
		if h, _, ok := strings.Cut(strings.TrimSpace(m), ":"); ok {
			n, _ := strconv.Atoi(h)
			wanted = append(wanted, n)
		}
	}
	if len(wanted) == 0 {
		return "", nil
	}
	public, mappings, err := s.Store.HostPortsInUse(ctx, sv.ID, d.ID)
	if err != nil {
		return "", err
	}
	used := map[int]string{sv.Port: "SSH"}
	for p, name := range public {
		used[p] = name
	}
	for name, m := range mappings {
		for _, x := range strings.Split(m, ",") {
			if h, _, ok := strings.Cut(strings.TrimSpace(x), ":"); ok {
				n, _ := strconv.Atoi(h)
				used[n] = name
			}
		}
	}
	for i, p := range wanted {
		if name, ok := used[p]; ok {
			return fmt.Sprintf("le port %d est déjà utilisé sur le serveur (%s)", p, name), nil
		}
		if slices.Contains(wanted[:i], p) {
			return fmt.Sprintf("le port %d est publié deux fois", p), nil
		}
	}
	return "", nil
}

// publicDatabase ajoute l'hôte interne et, si l'appelant peut lire les secrets, les identifiants et URLs de connexion.
func (s *Server) publicDatabase(ctx context.Context, d *store.Database, withSecrets bool) (map[string]any, error) {
	b, _ := json.Marshal(d)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	port := deploy.DBTypes[d.Type].Port
	m["internal_host"], m["internal_port"] = d.ID, port
	if !withSecrets {
		return m, nil
	}
	pass, err := s.Box.Decrypt(d.PasswordEnc)
	if err != nil {
		return nil, err
	}
	m["password"] = pass
	if d.RootPasswordEnc != "" {
		if m["root_password"], err = s.Box.Decrypt(d.RootPasswordEnc); err != nil {
			return nil, err
		}
	}
	m["internal_url"] = deploy.ConnectionURL(d, pass, d.ID, port)
	if d.IsPublic && d.PublicPort != nil {
		loc, err := s.Store.ResourceLocation(ctx, d.ID)
		if err != nil {
			return nil, err
		}
		sv, err := s.Store.ServerByIDAny(ctx, loc.ServerID)
		if err != nil {
			return nil, err
		}
		m["public_url"] = deploy.ConnectionURL(d, pass, sv.IP, *d.PublicPort)
	}
	return m, nil
}

// --- Handlers ---

func (s *Server) listDatabases(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	envID := r.URL.Query().Get("environment_id")
	if envID != "" && !isUUID(envID) {
		writeError(w, http.StatusBadRequest, "environment_id invalide")
		return
	}
	list, err := s.Store.ListDatabases(ctx, teamFrom(ctx).Team.ID, envID)
	if err != nil {
		s.internal(w, err)
		return
	}
	visible := []map[string]any{}
	for _, d := range list {
		if ok, err := s.can(ctx, "resource.read", resourceScope(d.ID)); err != nil {
			s.internal(w, err)
			return
		} else if ok {
			m, _ := s.publicDatabase(ctx, d, false)
			visible = append(visible, m)
		}
	}
	writeJSON(w, http.StatusOK, visible)
}

func (s *Server) createDatabase(w http.ResponseWriter, r *http.Request) {
	var in dbInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	team := teamFrom(ctx).Team
	if !isUUID(in.EnvironmentID) || !isUUID(in.DestinationID) {
		writeError(w, http.StatusUnprocessableEntity, "environment_id et destination_id requis")
		return
	}
	t, ok := deploy.DBTypes[in.Type]
	if !ok {
		writeError(w, http.StatusUnprocessableEntity, "type : postgresql, mysql, mariadb, mongodb, redis, keydb, dragonfly ou clickhouse")
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
	d := &store.Database{TeamID: team.ID, EnvironmentID: in.EnvironmentID, DestinationID: in.DestinationID, Type: in.Type,
		Image: t.Image, Username: t.User, DatabaseName: t.DB, InitScripts: []store.InitScript{}, Limits: store.DefaultLimits(),
		Settings: store.DefaultDatabaseSettings()}
	for _, x := range []struct {
		dst *string
		gen bool
	}{{&d.PasswordEnc, true}, {&d.RootPasswordEnc, t.Root}} {
		if x.gen {
			if *x.dst, err = s.Box.Encrypt(randomPassword(32)); err != nil {
				s.internal(w, err)
				return
			}
		}
	}
	if msg := in.apply(s, d); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if msg, err := s.checkHostPorts(ctx, d, sv); err != nil {
		s.internal(w, err)
		return
	} else if msg != "" {
		writeError(w, http.StatusConflict, msg)
		return
	}
	created, err := s.Store.CreateDatabase(ctx, d, func(id string) []*store.Storage { return deploy.DefaultDatabaseStorages(d.Type, id) })
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "database.created", "database", created.ID, map[string]any{"name": created.Name, "type": created.Type,
		"environment_id": created.EnvironmentID, "server_id": sv.ID})
	s.respondDatabase(w, r, created, http.StatusCreated)
}

func (s *Server) respondDatabase(w http.ResponseWriter, r *http.Request, d *store.Database, status int) {
	ctx := r.Context()
	secrets, err := s.can(ctx, "resource.env.read", resourceScope(d.ID))
	if err != nil {
		s.internal(w, err)
		return
	}
	m, err := s.publicDatabase(ctx, d, secrets)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, status, m)
}

// teamDatabase charge la base {db} de l'équipe et vérifie la permission sur sa portée.
func (s *Server) teamDatabase(w http.ResponseWriter, r *http.Request, perm string) (*store.Database, bool) {
	id, ok := idParam(w, r, "db")
	if !ok {
		return nil, false
	}
	d, err := s.Store.DatabaseByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return d, s.authorize(w, r, perm, resourceScope(d.ID))
}

func (s *Server) dbLoader(w http.ResponseWriter, r *http.Request, perm string) (string, string, string, bool) {
	d, ok := s.teamDatabase(w, r, perm)
	if !ok {
		return "", "", "", false
	}
	return d.ID, "database", d.Name, true
}

func (s *Server) getDatabase(w http.ResponseWriter, r *http.Request) {
	d, ok := s.teamDatabase(w, r, "resource.read")
	if !ok {
		return
	}
	ctx := r.Context()
	tags, err := s.Store.ResourceTags(ctx, d.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	loc, err := s.Store.ResourceLocation(ctx, d.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	secrets, err := s.can(ctx, "resource.env.read", resourceScope(d.ID))
	if err != nil {
		s.internal(w, err)
		return
	}
	m, err := s.publicDatabase(ctx, d, secrets)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"database": m, "tags": tags, "server_id": loc.ServerID, "project_id": loc.ProjectID})
}

func (s *Server) updateDatabase(w http.ResponseWriter, r *http.Request) {
	d, ok := s.teamDatabase(w, r, "resource.update")
	if !ok {
		return
	}
	var in dbInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	if (in.Password != nil || in.RootPassword != nil) && !s.authorize(w, r, "resource.env.write", resourceScope(d.ID)) {
		return
	}
	before := *d
	if msg := in.apply(s, d); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	loc, err := s.Store.ResourceLocation(ctx, d.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	sv, err := s.Store.ServerByIDAny(ctx, loc.ServerID)
	if err != nil {
		s.internal(w, err)
		return
	}
	if msg, err := s.checkHostPorts(ctx, d, sv); err != nil {
		s.internal(w, err)
		return
	} else if msg != "" {
		writeError(w, http.StatusConflict, msg)
		return
	}
	if err := s.Store.UpdateDatabase(ctx, d); err != nil {
		s.fail(w, err)
		return
	}
	meta := map[string]any{"name": d.Name, "password_changed": d.PasswordEnc != before.PasswordEnc,
		"root_password_changed": d.RootPasswordEnc != before.RootPasswordEnc}
	// Exposition publique : appliquée tout de suite (proxy), sans redémarrer la base.
	publicChanged := before.IsPublic != d.IsPublic || (d.IsPublic && !samePort(before.PublicPort, d.PublicPort))
	if publicChanged && d.Status != "stopped" && d.Status != "exited" && sv.IsUsable {
		task, err := s.enqueueDatabaseTask(ctx, d, deploy.TaskDatabaseProxy, &userFrom(ctx).ID)
		if err != nil {
			s.internal(w, err)
			return
		}
		meta["proxy_task_id"] = task.ID
	}
	s.auditTeam(r, "database.updated", "database", d.ID, meta)
	updated, err := s.Store.DatabaseByID(ctx, d.TeamID, d.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.respondDatabase(w, r, updated, http.StatusOK)
}

func samePort(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

// --- Suppression ---

type dbDeletePayload struct {
	TeamID        string `json:"team_id"`
	DatabaseID    string `json:"database_id"`
	DeleteVolumes bool   `json:"delete_volumes"`
}

func (s *Server) deleteDatabase(w http.ResponseWriter, r *http.Request) {
	d, ok := s.teamDatabase(w, r, "resource.delete")
	if !ok {
		return
	}
	delVolumes := r.URL.Query().Get("delete_volumes") == "true"
	req, err := s.gate(r, "resource.delete", resourceScope(d.ID), "Suppression de la base "+d.Name, dbDeletePayload{d.TeamID, d.ID, delVolumes})
	if err != nil {
		s.fail(w, err)
		return
	}
	if req != nil {
		respondPending(w, req)
		return
	}
	if err := s.removeDatabase(r.Context(), d, delVolumes); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "database.deleted", "database", d.ID, map[string]any{"name": d.Name, "delete_volumes": delVolumes})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeDatabase(ctx context.Context, d *store.Database, deleteVolumes bool) error {
	if s.Deployer != nil {
		if err := s.Deployer.TeardownDatabase(ctx, d, deleteVolumes); err != nil {
			return err
		}
	}
	return s.Store.DeleteDatabase(ctx, d.TeamID, d.ID)
}

// execDeleteResource : exécuteur de resource.delete (application ou base selon la demande).
func (s *Server) execDeleteResource(ctx context.Context, req *store.ApprovalRequest) error {
	var p dbDeletePayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	if p.DatabaseID == "" {
		return s.execDeleteApplication(ctx, req)
	}
	d, err := s.Store.DatabaseByID(ctx, p.TeamID, p.DatabaseID)
	if err != nil {
		return err
	}
	return s.removeDatabase(ctx, d, p.DeleteVolumes)
}

// --- Démarrage, arrêt, redémarrage ---

type dbActionPayload struct {
	TeamID     string  `json:"team_id"`
	DatabaseID string  `json:"database_id"`
	Kind       string  `json:"kind"`
	CreatedBy  *string `json:"created_by,omitempty"`
}

var errDatabaseBusy = errors.New("une action est déjà en cours sur cette base")

// enqueueDatabaseTask lance une action sur la base en tâche serveur.
func (s *Server) enqueueDatabaseTask(ctx context.Context, d *store.Database, kind string, by *string) (*store.ServerTask, error) {
	if busy, err := s.Store.HasActiveDatabaseTask(ctx, d.ID); err != nil {
		return nil, err
	} else if busy && kind != deploy.TaskDatabaseProxy {
		return nil, errDatabaseBusy
	}
	loc, err := s.Store.ResourceLocation(ctx, d.ID)
	if err != nil {
		return nil, err
	}
	params, _ := json.Marshal(deploy.DatabaseParams{DatabaseID: d.ID})
	return s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: d.TeamID, ServerID: loc.ServerID, Kind: kind, Params: params, CreatedBy: by})
}

func (s *Server) execDatabaseAction(ctx context.Context, req *store.ApprovalRequest) error {
	var p dbActionPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	d, err := s.Store.DatabaseByID(ctx, p.TeamID, p.DatabaseID)
	if err != nil {
		return err
	}
	_, err = s.enqueueDatabaseTask(ctx, d, p.Kind, p.CreatedBy)
	return err
}

// execResourceDeploy : exécuteur de resource.deploy (déploiement d'application ou action sur une base).
func (s *Server) execResourceDeploy(ctx context.Context, req *store.ApprovalRequest) error {
	var p struct {
		DatabaseID string `json:"database_id"`
	}
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	if p.DatabaseID != "" {
		return s.execDatabaseAction(ctx, req)
	}
	return s.execDeploy(ctx, req)
}

// databaseAction : start / restart (soumis aux approbations) et stop.
func (s *Server) databaseAction(kind string) http.HandlerFunc {
	verbs := map[string]string{deploy.TaskDatabaseStart: "Démarrage", deploy.TaskDatabaseRestart: "Redémarrage", deploy.TaskDatabaseStop: "Arrêt"}
	return func(w http.ResponseWriter, r *http.Request) {
		d, ok := s.teamDatabase(w, r, "resource.deploy")
		if !ok {
			return
		}
		ctx := r.Context()
		loc, err := s.Store.ResourceLocation(ctx, d.ID)
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
		if kind != deploy.TaskDatabaseStop {
			pending, err := s.gate(r, "resource.deploy", resourceScope(d.ID), verbs[kind]+" de la base "+d.Name,
				dbActionPayload{d.TeamID, d.ID, kind, &uid})
			if err != nil {
				s.fail(w, err)
				return
			}
			if pending != nil {
				respondPending(w, pending)
				return
			}
		}
		task, err := s.enqueueDatabaseTask(ctx, d, kind, &uid)
		if errors.Is(err, errDatabaseBusy) {
			writeError(w, http.StatusConflict, err.Error())
			return
		} else if err != nil {
			s.internal(w, err)
			return
		}
		s.auditTeam(r, "database."+strings.TrimPrefix(kind, "database_"), "database", d.ID, map[string]string{"task_id": task.ID})
		writeJSON(w, http.StatusAccepted, task)
	}
}

func (s *Server) databaseTasks(w http.ResponseWriter, r *http.Request) {
	d, ok := s.teamDatabase(w, r, "resource.logs.read")
	if !ok {
		return
	}
	list, err := s.Store.DatabaseTasks(r.Context(), d.ID, 50)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) databaseLogs(w http.ResponseWriter, r *http.Request) {
	d, ok := s.teamDatabase(w, r, "resource.logs.read")
	if !ok || !s.deployer(w) {
		return
	}
	lines, _ := strconv.Atoi(r.URL.Query().Get("lines"))
	if lines <= 0 || lines > 5000 {
		lines = 100
	}
	out, err := s.Deployer.DatabaseLogs(r.Context(), d, lines)
	if err != nil {
		writeError(w, http.StatusBadGateway, "serveur inaccessible : "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"logs": out})
}

func (s *Server) setDatabaseTags(w http.ResponseWriter, r *http.Request) {
	d, ok := s.teamDatabase(w, r, "resource.update")
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
	if err := s.Store.SetResourceTags(r.Context(), d.TeamID, "database", d.ID, in.TagIDs); err != nil {
		s.internal(w, err)
		return
	}
	tags, err := s.Store.ResourceTags(r.Context(), d.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "database.tags.updated", "database", d.ID, in)
	writeJSON(w, http.StatusOK, tags)
}
