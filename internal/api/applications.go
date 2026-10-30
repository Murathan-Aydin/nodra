package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	"github.com/Murathan-Aydin/nodra/internal/deploy"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

var (
	branchRe     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,254}$`)
	shaRe        = regexp.MustCompile(`^(HEAD|[0-9a-fA-F]{7,40})$`)
	imageRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9._/:@-]{0,254}$`)
	imageTagRe   = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	sizeRe       = regexp.MustCompile(`^\d+(\.\d+)?[bkmgBKMG]?$`)
	cpuRe        = regexp.MustCompile(`^\d+(\.\d+)?$`)
	cpusetRe     = regexp.MustCompile(`^[0-9,-]*$`)
	sshRepoRe    = regexp.MustCompile(`^(ssh://)?[A-Za-z0-9._-]+@[A-Za-z0-9.-]+(:[0-9]+)?[:/][A-Za-z0-9._/~-]+$`)
	volumeNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
	serviceRe    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,62}$`)
	urlPathRe    = regexp.MustCompile(`^(/[A-Za-z0-9._~-]+)*/?$`)
	portRe       = regexp.MustCompile(`^\d{1,5}$`)
)

var buildPacks = []string{"nixpacks", "dockerfile", "dockercompose", "dockerimage", "static"}

func resourceScope(id string) rbac.Scope { return rbac.Scope{Type: rbac.ScopeResource, ID: id} }

type appInput struct {
	EnvironmentID           string              `json:"environment_id"`
	DestinationID           string              `json:"destination_id"`
	Name                    *string             `json:"name"`
	Description             *string             `json:"description"`
	GitRepository           *string             `json:"git_repository"`
	GitBranch               *string             `json:"git_branch"`
	GitCommitSHA            *string             `json:"git_commit_sha"`
	SourceID                *string             `json:"source_id"`
	PrivateKeyID            *string             `json:"private_key_id"`
	BuildPack               *string             `json:"build_pack"`
	DockerImage             *string             `json:"docker_image"`
	DockerImageTag          *string             `json:"docker_image_tag"`
	StaticImage             *string             `json:"static_image"`
	BaseDirectory           *string             `json:"base_directory"`
	PublishDirectory        *string             `json:"publish_directory"`
	Dockerfile              *string             `json:"dockerfile"`
	DockerfileLocation      *string             `json:"dockerfile_location"`
	DockerfileTarget        *string             `json:"dockerfile_target"`
	DockerComposeLocation   *string             `json:"docker_compose_location"`
	DockerComposeRaw        *string             `json:"docker_compose_raw"`
	DockerComposeDomains    map[string][]string `json:"docker_compose_domains"`
	InstallCommand          *string             `json:"install_command"`
	BuildCommand            *string             `json:"build_command"`
	StartCommand            *string             `json:"start_command"`
	WatchPaths              *string             `json:"watch_paths"`
	PortsExposes            *string             `json:"ports_exposes"`
	PortsMappings           *string             `json:"ports_mappings"`
	Domains                 *[]string           `json:"domains"`
	Redirect                *string             `json:"redirect"`
	PreviewURLTemplate      *string             `json:"preview_url_template"`
	CustomLabels            *string             `json:"custom_labels"`
	CustomDockerRunOptions  *string             `json:"custom_docker_run_options"`
	PreDeploymentCommand    *string             `json:"pre_deployment_command"`
	PreDeploymentContainer  *string             `json:"pre_deployment_container"`
	PostDeploymentCommand   *string             `json:"post_deployment_command"`
	PostDeploymentContainer *string             `json:"post_deployment_container"`
	HealthCheck             json.RawMessage     `json:"health_check"`
	Limits                  json.RawMessage     `json:"limits"`
	Settings                json.RawMessage     `json:"settings"`
	BasicAuthPassword       *string             `json:"basic_auth_password"`
	ForceDomainOverride     bool                `json:"force_domain_override"`
}

func validRepoPath(p string) bool {
	return strings.HasPrefix(p, "/") && !strings.Contains(p, "..") && !strings.ContainsAny(p, "\x00\n\r") && len(p) <= 255
}

// NormalizeAppDomain valide un domaine d'application (schéma http/https, chemin facultatif) et le normalise.
func NormalizeAppDomain(raw string) (normalized, host string, ok bool) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", false
	}
	host = strings.ToLower(u.Hostname())
	if !strings.Contains(host, ".") || (!hostnameRe.MatchString(host) && net.ParseIP(host) == nil) {
		return "", "", false
	}
	if p := u.Port(); p != "" && !portRe.MatchString(p) {
		return "", "", false
	}
	if !urlPathRe.MatchString(u.Path) {
		return "", "", false
	}
	out := u.Scheme + "://" + strings.ToLower(u.Host) + strings.TrimSuffix(u.Path, "/")
	return out, host, true
}

func validPorts(s string, mapping bool) bool {
	if s == "" {
		return true
	}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		parts := []string{p}
		if mapping {
			if parts = strings.Split(p, ":"); len(parts) != 2 {
				return false
			}
		}
		for _, x := range parts {
			var n int
			if _, err := fmt.Sscanf(x, "%d", &n); err != nil || !portRe.MatchString(x) || n < 1 || n > 65535 {
				return false
			}
		}
	}
	return true
}

// apply fusionne l'entrée dans l'application et valide les champs indépendants de la base.
func (in *appInput) apply(a *store.Application) string {
	str := func(dst *string, v *string) {
		if v != nil {
			*dst = strings.TrimSpace(*v)
		}
	}
	for _, f := range []struct {
		dst *string
		v   *string
	}{
		{&a.Name, in.Name}, {&a.Description, in.Description}, {&a.GitRepository, in.GitRepository}, {&a.GitBranch, in.GitBranch},
		{&a.GitCommitSHA, in.GitCommitSHA}, {&a.BuildPack, in.BuildPack}, {&a.DockerImage, in.DockerImage},
		{&a.DockerImageTag, in.DockerImageTag}, {&a.StaticImage, in.StaticImage}, {&a.BaseDirectory, in.BaseDirectory},
		{&a.PublishDirectory, in.PublishDirectory}, {&a.DockerfileLocation, in.DockerfileLocation},
		{&a.DockerfileTarget, in.DockerfileTarget}, {&a.DockerComposeLocation, in.DockerComposeLocation},
		{&a.InstallCommand, in.InstallCommand}, {&a.BuildCommand, in.BuildCommand}, {&a.StartCommand, in.StartCommand},
		{&a.WatchPaths, in.WatchPaths}, {&a.PortsExposes, in.PortsExposes}, {&a.PortsMappings, in.PortsMappings},
		{&a.Redirect, in.Redirect}, {&a.PreviewURLTemplate, in.PreviewURLTemplate}, {&a.CustomDockerRunOptions, in.CustomDockerRunOptions},
		{&a.PreDeploymentCommand, in.PreDeploymentCommand}, {&a.PreDeploymentContainer, in.PreDeploymentContainer},
		{&a.PostDeploymentCommand, in.PostDeploymentCommand}, {&a.PostDeploymentContainer, in.PostDeploymentContainer},
	} {
		str(f.dst, f.v)
	}
	// Contenus multi-lignes conservés tels quels.
	if in.Dockerfile != nil {
		a.Dockerfile = *in.Dockerfile
	}
	if in.DockerComposeRaw != nil {
		a.DockerComposeRaw = *in.DockerComposeRaw
	}
	if in.CustomLabels != nil {
		a.CustomLabels = *in.CustomLabels
	}
	if in.DockerComposeDomains != nil {
		a.DockerComposeDomains = in.DockerComposeDomains
	}
	if in.SourceID != nil {
		a.SourceID = nilIfEmpty(*in.SourceID)
	}
	if in.PrivateKeyID != nil {
		a.PrivateKeyID = nilIfEmpty(*in.PrivateKeyID)
	}
	hash := a.Settings.BasicAuthPasswordHash // jamais fourni directement par le client
	defer func() {
		if in.BasicAuthPassword == nil {
			a.Settings.BasicAuthPasswordHash = hash
		}
	}()
	for _, x := range []struct {
		raw json.RawMessage
		dst any
		nm  string
	}{{in.HealthCheck, &a.HealthCheck, "health_check"}, {in.Limits, &a.Limits, "limits"}, {in.Settings, &a.Settings, "settings"}} {
		if len(x.raw) > 0 {
			if err := json.Unmarshal(x.raw, x.dst); err != nil {
				return x.nm + " invalide : " + err.Error()
			}
		}
	}
	if in.BasicAuthPassword != nil {
		if *in.BasicAuthPassword == "" {
			a.Settings.BasicAuthPasswordHash = ""
		} else {
			h, err := bcrypt.GenerateFromPassword([]byte(*in.BasicAuthPassword), 10)
			if err != nil {
				return "mot de passe invalide"
			}
			a.Settings.BasicAuthPasswordHash = string(h)
		}
	}
	if in.Domains != nil {
		seen := map[string]bool{}
		a.Domains = []string{}
		for _, d := range *in.Domains {
			if strings.TrimSpace(d) == "" {
				continue
			}
			n, _, ok := NormalizeAppDomain(d)
			if !ok {
				return "domaine invalide : " + d
			}
			if !seen[n] {
				seen[n] = true
				a.Domains = append(a.Domains, n)
			}
		}
	}
	if in.BasicAuthPassword == nil {
		a.Settings.BasicAuthPasswordHash = hash
	}
	return validateApp(a)
}

func nilIfEmpty(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	s = strings.TrimSpace(s)
	return &s
}

func validateApp(a *store.Application) string {
	hc, lim, st := a.HealthCheck, a.Limits, a.Settings
	needsRepo := !(a.BuildPack == "dockerimage" || (a.BuildPack == "dockerfile" && strings.TrimSpace(a.Dockerfile) != "") ||
		(a.BuildPack == "dockercompose" && strings.TrimSpace(a.DockerComposeRaw) != ""))
	switch {
	case a.Name == "" || len(a.Name) > 100:
		return "nom requis (100 caractères max.)"
	case !slices.Contains(buildPacks, a.BuildPack):
		return "build_pack : nixpacks, dockerfile, dockercompose, dockerimage ou static"
	case needsRepo && a.GitRepository == "":
		return "git_repository requis pour ce build pack"
	case a.GitRepository != "" && (!branchRe.MatchString(a.GitBranch) || strings.Contains(a.GitBranch, "..")):
		return "git_branch invalide"
	case !shaRe.MatchString(a.GitCommitSHA):
		return "git_commit_sha : HEAD ou un SHA hexadécimal"
	case a.BuildPack == "dockerimage" && !imageRe.MatchString(a.DockerImage):
		return "docker_image invalide"
	case a.DockerImageTag != "" && !imageTagRe.MatchString(a.DockerImageTag):
		return "docker_image_tag invalide"
	case a.BuildPack == "static" && !imageRe.MatchString(a.StaticImage):
		return "static_image invalide"
	case !validRepoPath(a.BaseDirectory) || !validRepoPath(a.DockerfileLocation) || !validRepoPath(a.DockerComposeLocation):
		return "chemins du dépôt : absolus (commençant par /) et sans .."
	case a.PublishDirectory != "" && !validRepoPath(a.PublishDirectory):
		return "publish_directory : chemin absolu sans .."
	case a.DockerfileTarget != "" && !serviceRe.MatchString(a.DockerfileTarget):
		return "dockerfile_target invalide"
	case !validPorts(a.PortsExposes, false):
		return "ports_exposes : liste de ports séparés par des virgules (ex. 3000,8080)"
	case !validPorts(a.PortsMappings, true):
		return "ports_mappings : liste hôte:conteneur (ex. 8080:80)"
	case !slices.Contains([]string{"both", "www", "non-www"}, a.Redirect):
		return "redirect : both, www ou non-www"
	case !strings.Contains(a.PreviewURLTemplate, "{{pr_id}}"):
		return "preview_url_template doit contenir {{pr_id}}"
	case a.PreDeploymentContainer != "" && !serviceRe.MatchString(a.PreDeploymentContainer),
		a.PostDeploymentContainer != "" && !serviceRe.MatchString(a.PostDeploymentContainer):
		return "conteneur des commandes pré/post-déploiement invalide"
	case !slices.Contains([]string{"http", "cmd"}, hc.Type) || !slices.Contains([]string{"GET", "POST", "HEAD"}, hc.Method) ||
		!slices.Contains([]string{"http", "https"}, hc.Scheme):
		return "health_check : type http|cmd, method GET|POST|HEAD, scheme http|https"
	case hc.Port != "" && !validPorts(hc.Port, false), !strings.HasPrefix(hc.Path, "/"), hc.Interval < 1, hc.Timeout < 1, hc.Retries < 1, hc.StartPeriod < 0,
		hc.ReturnCode < 100 || hc.ReturnCode > 599, hc.Type == "cmd" && hc.Command == "", !hostnameRe.MatchString(hc.Host):
		return "health_check : valeurs invalides"
	case !sizeRe.MatchString(lim.Memory) || !sizeRe.MatchString(lim.MemorySwap) || !sizeRe.MatchString(lim.MemoryReservation),
		!cpuRe.MatchString(lim.CPUs), !cpusetRe.MatchString(lim.CPUSet), lim.MemorySwappiness < 0 || lim.MemorySwappiness > 100, lim.CPUShares < 0:
		return "limits : valeurs invalides (ex. memory \"512m\", cpus \"1.5\")"
	case st.ImagesToKeep < 0 || st.ImagesToKeep > 50, st.StopGracePeriod < 0 || st.StopGracePeriod > 3600:
		return "settings : docker_images_to_keep (0-50) ou stop_grace_period (0-3600) invalide"
	case st.CustomInternalName != "" && !serviceRe.MatchString(st.CustomInternalName):
		return "settings.custom_internal_name invalide"
	case st.BasicAuthEnabled && (st.BasicAuthUsername == "" || strings.ContainsAny(st.BasicAuthUsername, ":$\n") || st.BasicAuthPasswordHash == ""):
		return "authentification HTTP : basic_auth_username et basic_auth_password requis"
	}
	if strings.TrimSpace(a.DockerComposeRaw) != "" {
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(a.DockerComposeRaw), &doc); err != nil || doc["services"] == nil {
			return "docker_compose_raw : YAML invalide ou sans services"
		}
	}
	for svc, ds := range a.DockerComposeDomains {
		if !serviceRe.MatchString(svc) {
			return "docker_compose_domains : service invalide " + svc
		}
		for i, d := range ds {
			n, _, ok := NormalizeAppDomain(d)
			if !ok {
				return "docker_compose_domains : domaine invalide " + d
			}
			ds[i] = n
		}
	}
	return ""
}

// checkAppRefs vérifie la source, la clé et le format du dépôt (dépendent de la base).
func (s *Server) checkAppRefs(ctx context.Context, a *store.Application) string {
	team := teamFrom(ctx).Team.ID
	if a.SourceID != nil && a.PrivateKeyID != nil {
		return "choisissez une source Git ou une deploy key, pas les deux"
	}
	if a.SourceID != nil {
		if !isUUID(*a.SourceID) {
			return "source inconnue"
		}
		if _, err := s.Store.GitSourceByID(ctx, team, *a.SourceID); err != nil {
			return "source inconnue"
		}
		if strings.Contains(a.GitRepository, "://") || strings.Count(a.GitRepository, "/") < 1 {
			return "avec une source, git_repository est de la forme owner/nom"
		}
	}
	if a.PrivateKeyID != nil {
		if !isUUID(*a.PrivateKeyID) {
			return "clé privée inconnue"
		}
		if _, err := s.Store.PrivateKeyByID(ctx, team, *a.PrivateKeyID); err != nil {
			return "clé privée inconnue"
		}
		if !sshRepoRe.MatchString(a.GitRepository) {
			return "avec une deploy key, git_repository est une URL SSH (git@hôte:owner/nom.git)"
		}
	}
	if a.SourceID == nil && a.PrivateKeyID == nil && a.GitRepository != "" {
		if u, err := url.Parse(a.GitRepository); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil {
			return "dépôt public : URL https complète (ex. https://github.com/owner/nom)"
		}
	}
	return ""
}

// checkDomains signale les domaines déjà utilisés par une autre ressource.
func (s *Server) checkDomains(ctx context.Context, a *store.Application) (string, error) {
	var hosts []string
	all := append([]string{}, a.Domains...)
	for _, ds := range a.DockerComposeDomains {
		all = append(all, ds...)
	}
	for _, d := range all {
		if _, h, ok := NormalizeAppDomain(d); ok {
			hosts = append(hosts, h)
		}
	}
	if len(hosts) == 0 {
		return "", nil
	}
	owners, err := s.Store.DomainOwners(ctx, hosts, a.ID)
	if err != nil {
		return "", err
	}
	for h, name := range owners {
		return fmt.Sprintf("le domaine %s est déjà utilisé par %s (force_domain_override pour passer outre)", h, name), nil
	}
	return "", nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// defaultDomain génère un domaine : sous-domaine du wildcard du serveur, sinon sslip.io.
func defaultDomain(name string, sv *store.Server) string {
	slug := strings.Trim(slugRe.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(slug) > 40 {
		slug = slug[:40]
	}
	slug += "-" + randomHex(3)
	if sv.Settings.WildcardDomain != "" {
		u, _ := url.Parse(sv.Settings.WildcardDomain)
		return u.Scheme + "://" + slug + "." + u.Host
	}
	if net.ParseIP(sv.IP) != nil {
		return "http://" + slug + "." + sv.IP + ".sslip.io"
	}
	return ""
}

// publicApp masque le hash du mot de passe d'authentification HTTP.
func publicApp(a *store.Application) map[string]any {
	cp := *a
	set := cp.Settings.BasicAuthPasswordHash != ""
	cp.Settings.BasicAuthPasswordHash = ""
	b, _ := json.Marshal(cp)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["basic_auth_password_set"] = set
	return m
}

// --- Handlers ---

func (s *Server) listApplications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	envID := r.URL.Query().Get("environment_id")
	if envID != "" && !isUUID(envID) {
		writeError(w, http.StatusBadRequest, "environment_id invalide")
		return
	}
	list, err := s.Store.ListApplications(ctx, teamFrom(ctx).Team.ID, envID)
	if err != nil {
		s.internal(w, err)
		return
	}
	visible := []map[string]any{}
	for _, a := range list {
		if ok, err := s.can(ctx, "resource.read", resourceScope(a.ID)); err != nil {
			s.internal(w, err)
			return
		} else if ok {
			visible = append(visible, publicApp(a))
		}
	}
	writeJSON(w, http.StatusOK, visible)
}

func (s *Server) createApplication(w http.ResponseWriter, r *http.Request) {
	var in appInput
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
	a := &store.Application{TeamID: team.ID, EnvironmentID: in.EnvironmentID, DestinationID: in.DestinationID, GitBranch: "main",
		GitCommitSHA: "HEAD", StaticImage: "nginx:alpine", BaseDirectory: "/", DockerfileLocation: "/Dockerfile",
		DockerComposeLocation: "/docker-compose.yaml", Redirect: "both", PreviewURLTemplate: "{{pr_id}}.{{domain}}",
		DockerComposeDomains: map[string][]string{}, Domains: []string{},
		HealthCheck: store.DefaultHealthCheck(), Limits: store.DefaultLimits(), Settings: store.DefaultAppSettings()}
	if msg := in.apply(a); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if a.PortsExposes == "" {
		a.PortsExposes = map[string]string{"nixpacks": "3000", "static": "80", "dockerfile": "80", "dockerimage": "80"}[a.BuildPack]
	}
	if msg := s.checkAppRefs(ctx, a); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if in.Domains == nil && a.BuildPack != "dockercompose" {
		if d := defaultDomain(a.Name, sv); d != "" {
			a.Domains = []string{d}
		}
	}
	if !in.ForceDomainOverride {
		if msg, err := s.checkDomains(ctx, a); err != nil {
			s.internal(w, err)
			return
		} else if msg != "" {
			writeError(w, http.StatusConflict, msg)
			return
		}
	}
	if a.WebhookSecretEnc, err = s.Box.Encrypt(randomHex(32)); err != nil {
		s.internal(w, err)
		return
	}
	created, err := s.Store.CreateApplication(ctx, a)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "application.created", "application", created.ID, map[string]any{"name": created.Name, "build_pack": created.BuildPack,
		"environment_id": created.EnvironmentID, "server_id": sv.ID})
	writeJSON(w, http.StatusCreated, publicApp(created))
}

// teamApp charge l'application {app} de l'équipe et vérifie la permission sur sa portée.
func (s *Server) teamApp(w http.ResponseWriter, r *http.Request, perm string) (*store.Application, bool) {
	id, ok := idParam(w, r, "app")
	if !ok {
		return nil, false
	}
	a, err := s.Store.ApplicationByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return a, s.authorize(w, r, perm, resourceScope(a.ID))
}

func (s *Server) getApplication(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.read")
	if !ok {
		return
	}
	tags, err := s.Store.ResourceTags(r.Context(), a.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	loc, err := s.Store.ResourceLocation(r.Context(), a.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"application": publicApp(a), "tags": tags, "server_id": loc.ServerID, "project_id": loc.ProjectID})
}

func (s *Server) updateApplication(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.update")
	if !ok {
		return
	}
	var in appInput
	if !decode(w, r, &in) {
		return
	}
	before := *a
	if msg := in.apply(a); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	ctx := r.Context()
	if msg := s.checkAppRefs(ctx, a); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	// Conflits vérifiés seulement si les domaines changent (un conflit forcé auparavant ne bloque pas les autres modifications).
	if !in.ForceDomainOverride && (in.Domains != nil || in.DockerComposeDomains != nil) {
		if msg, err := s.checkDomains(ctx, a); err != nil {
			s.internal(w, err)
			return
		} else if msg != "" {
			writeError(w, http.StatusConflict, msg)
			return
		}
	}
	if err := s.Store.UpdateApplication(ctx, a); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "application.updated", "application", a.ID, map[string]any{"before": publicApp(&before), "after": publicApp(a)})
	updated, err := s.Store.ApplicationByID(ctx, a.TeamID, a.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, publicApp(updated))
}

type appPayload struct {
	TeamID        string `json:"team_id"`
	AppID         string `json:"application_id"`
	DeleteVolumes bool   `json:"delete_volumes"`
}

func (s *Server) deleteApplication(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.delete")
	if !ok {
		return
	}
	delVolumes := r.URL.Query().Get("delete_volumes") == "true"
	req, err := s.gate(r, "resource.delete", resourceScope(a.ID), "Suppression de l'application "+a.Name, appPayload{a.TeamID, a.ID, delVolumes})
	if err != nil {
		s.fail(w, err)
		return
	}
	if req != nil {
		respondPending(w, req)
		return
	}
	if err := s.removeApplication(r.Context(), a, delVolumes); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "application.deleted", "application", a.ID, map[string]any{"name": a.Name, "delete_volumes": delVolumes})
	w.WriteHeader(http.StatusNoContent)
}

// removeApplication arrête les conteneurs de l'application (si un exécuteur est branché) puis la supprime.
func (s *Server) removeApplication(ctx context.Context, a *store.Application, deleteVolumes bool) error {
	if s.Deployer != nil {
		if err := s.Deployer.Teardown(ctx, a, deleteVolumes); err != nil {
			return err
		}
	}
	return s.Store.DeleteApplication(ctx, a.TeamID, a.ID)
}

func (s *Server) execDeleteApplication(ctx context.Context, req *store.ApprovalRequest) error {
	var p appPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	a, err := s.Store.ApplicationByID(ctx, p.TeamID, p.AppID)
	if err != nil {
		return err
	}
	return s.removeApplication(ctx, a, p.DeleteVolumes)
}

// --- Webhooks manuels ---

func (s *Server) appWebhooks(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.update")
	if !ok {
		return
	}
	if r.Method == http.MethodPost { // rotation du secret
		enc, err := s.Box.Encrypt(randomHex(32))
		if err != nil {
			s.internal(w, err)
			return
		}
		a.WebhookSecretEnc = enc
		if err := s.Store.UpdateApplication(r.Context(), a); err != nil {
			s.internal(w, err)
			return
		}
		s.auditTeam(r, "application.webhook_secret.rotated", "application", a.ID, nil)
	}
	secret, err := s.Box.Decrypt(a.WebhookSecretEnc)
	if err != nil {
		s.internal(w, err)
		return
	}
	base := s.baseURL(r) + "/webhooks/"
	writeJSON(w, http.StatusOK, map[string]any{"secret": secret, "urls": map[string]string{
		"github": base + "github/manual", "gitlab": base + "gitlab/manual", "gitea": base + "gitea/manual",
		"bitbucket": base + "bitbucket/manual", "deploy": base + "deploy/" + a.ID,
	}})
}

// --- Tags ---

func (s *Server) setApplicationTags(w http.ResponseWriter, r *http.Request) {
	a, ok := s.teamApp(w, r, "resource.update")
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
	if err := s.Store.SetResourceTags(r.Context(), a.TeamID, "application", a.ID, in.TagIDs); err != nil {
		s.internal(w, err)
		return
	}
	tags, err := s.Store.ResourceTags(r.Context(), a.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "application.tags.updated", "application", a.ID, in)
	writeJSON(w, http.StatusOK, tags)
}

// Deployer pilote les conteneurs des applications (implémenté par deploy.Runner).
type Deployer interface {
	Teardown(ctx context.Context, a *store.Application, deleteVolumes bool) error
	Stop(ctx context.Context, a *store.Application, pr int) error
	ContainerLogs(ctx context.Context, a *store.Application, pr, lines int) (string, error)
	Images(ctx context.Context, a *store.Application) ([]deploy.Image, error)
}
