package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
)

func init() {
	scopeParents[rbac.ScopeResource] = func(ctx context.Context, s *Store, id string) (rbac.Scope, error) {
		var env string
		err := s.Pool.QueryRow(ctx, `SELECT environment_id::text FROM resources WHERE id = $1`, id).Scan(&env)
		if errors.Is(err, pgx.ErrNoRows) {
			return rbac.Scope{}, ErrNotFound
		}
		return rbac.Scope{Type: rbac.ScopeEnvironment, ID: env}, err
	}
}

// HealthCheck : vérification de santé du conteneur (valeurs par défaut de Coolify).
type HealthCheck struct {
	Enabled     bool   `json:"enabled"`
	Type        string `json:"type"` // http | cmd
	Path        string `json:"path"`
	Port        string `json:"port"` // vide = premier port exposé
	Host        string `json:"host"`
	Method      string `json:"method"`
	ReturnCode  int    `json:"return_code"`
	Scheme      string `json:"scheme"`
	Response    string `json:"response_text"`
	Interval    int    `json:"interval"`
	Timeout     int    `json:"timeout"`
	Retries     int    `json:"retries"`
	StartPeriod int    `json:"start_period"`
	Command     string `json:"command"`
}

func DefaultHealthCheck() HealthCheck {
	return HealthCheck{Type: "http", Path: "/", Host: "localhost", Method: "GET", ReturnCode: 200, Scheme: "http",
		Interval: 5, Timeout: 5, Retries: 10, StartPeriod: 5}
}

// Limits : limites de ressources du conteneur ("0" = illimité).
type Limits struct {
	Memory            string `json:"memory"`
	MemorySwap        string `json:"memory_swap"`
	MemorySwappiness  int    `json:"memory_swappiness"`
	MemoryReservation string `json:"memory_reservation"`
	CPUs              string `json:"cpus"`
	CPUSet            string `json:"cpuset"`
	CPUShares         int    `json:"cpu_shares"`
}

func DefaultLimits() Limits {
	return Limits{Memory: "0", MemorySwap: "0", MemorySwappiness: 60, MemoryReservation: "0", CPUs: "0", CPUShares: 1024}
}

// AppSettings : options d'une application (équivalent de ApplicationSetting dans Coolify).
type AppSettings struct {
	AutoDeploy                bool   `json:"auto_deploy"`
	ForceHTTPS                bool   `json:"force_https"`
	PreviewDeployments        bool   `json:"preview_deployments"`
	PublicPRDeployments       bool   `json:"public_pr_deployments"` // déployer aussi les PR venant de forks
	GitSubmodules             bool   `json:"git_submodules"`
	GitLFS                    bool   `json:"git_lfs"`
	GitShallowClone           bool   `json:"git_shallow_clone"`
	IsSPA                     bool   `json:"is_spa"`
	Gzip                      bool   `json:"gzip"`
	StripPrefix               bool   `json:"strip_prefix"`
	ConsistentContainerName   bool   `json:"consistent_container_name"`
	ConnectToDockerNetwork    bool   `json:"connect_to_docker_network"`
	CustomInternalName        string `json:"custom_internal_name"`
	DisableBuildCache         bool   `json:"disable_build_cache"`
	IncludeSourceCommit       bool   `json:"include_source_commit_in_build"`
	InjectBuildArgs           bool   `json:"inject_build_args"`
	UseBuildSecrets           bool   `json:"use_build_secrets"`
	IncludeDefaultVariables   bool   `json:"include_default_variables"`
	ImagesToKeep              int    `json:"docker_images_to_keep"`
	StopGracePeriod           int    `json:"stop_grace_period"`
	Debug                     bool   `json:"debug"`
	GPUEnabled                bool   `json:"gpu_enabled"`
	GPUDriver                 string `json:"gpu_driver"`
	GPUCount                  string `json:"gpu_count"`
	GPUDeviceIDs              string `json:"gpu_device_ids"`
	BasicAuthEnabled          bool   `json:"basic_auth_enabled"`
	BasicAuthUsername         string `json:"basic_auth_username"`
	BasicAuthPasswordHash     string `json:"basic_auth_password_hash,omitempty"` // bcrypt (htpasswd)
	CertificateResolver       string `json:"certificate_resolver"`
	RawComposeDeployment      bool   `json:"raw_compose_deployment"`
	PreserveRepository        bool   `json:"preserve_repository"`
	RedactEnvValuesInLogs     bool   `json:"redact_env_values_in_logs"`
	PermanentRedirect         bool   `json:"permanent_redirect"`
	ContainerLabelEscape      bool   `json:"container_label_escape"`
	IncludeTimestampsInLogs   bool   `json:"include_timestamps"`
	BuildServerEnabled        bool   `json:"build_server_enabled"`
	EnvSorting                bool   `json:"env_sorting"`
	ContainerLabelReadonly    bool   `json:"container_label_readonly"`
	ComposeParsingVersion     int    `json:"compose_parsing_version"`
	RollingUpdateDrainSeconds int    `json:"rolling_update_drain_seconds"`
}

func DefaultAppSettings() AppSettings {
	return AppSettings{AutoDeploy: true, ForceHTTPS: true, GitSubmodules: true, GitLFS: true, GitShallowClone: true, Gzip: true,
		StripPrefix: true, InjectBuildArgs: true, IncludeDefaultVariables: true, ImagesToKeep: 2, StopGracePeriod: 30,
		CertificateResolver: "letsencrypt", ContainerLabelEscape: true, ContainerLabelReadonly: true, ComposeParsingVersion: 1}
}

type Application struct {
	ID                      string              `json:"id"`
	TeamID                  string              `json:"team_id"`
	EnvironmentID           string              `json:"environment_id"`
	DestinationID           string              `json:"destination_id"`
	Name                    string              `json:"name"`
	Description             string              `json:"description"`
	Status                  string              `json:"status"`
	GitRepository           string              `json:"git_repository"`
	GitBranch               string              `json:"git_branch"`
	GitCommitSHA            string              `json:"git_commit_sha"`
	SourceID                *string             `json:"source_id"`
	PrivateKeyID            *string             `json:"private_key_id"`
	BuildPack               string              `json:"build_pack"`
	DockerImage             string              `json:"docker_image"`
	DockerImageTag          string              `json:"docker_image_tag"`
	StaticImage             string              `json:"static_image"`
	BaseDirectory           string              `json:"base_directory"`
	PublishDirectory        string              `json:"publish_directory"`
	Dockerfile              string              `json:"dockerfile"`
	DockerfileLocation      string              `json:"dockerfile_location"`
	DockerfileTarget        string              `json:"dockerfile_target"`
	DockerComposeLocation   string              `json:"docker_compose_location"`
	DockerComposeRaw        string              `json:"docker_compose_raw"`
	DockerComposeDomains    map[string][]string `json:"docker_compose_domains"`
	InstallCommand          string              `json:"install_command"`
	BuildCommand            string              `json:"build_command"`
	StartCommand            string              `json:"start_command"`
	WatchPaths              string              `json:"watch_paths"`
	PortsExposes            string              `json:"ports_exposes"`
	PortsMappings           string              `json:"ports_mappings"`
	Domains                 []string            `json:"domains"`
	Redirect                string              `json:"redirect"`
	PreviewURLTemplate      string              `json:"preview_url_template"`
	CustomLabels            string              `json:"custom_labels"`
	CustomDockerRunOptions  string              `json:"custom_docker_run_options"`
	PreDeploymentCommand    string              `json:"pre_deployment_command"`
	PreDeploymentContainer  string              `json:"pre_deployment_container"`
	PostDeploymentCommand   string              `json:"post_deployment_command"`
	PostDeploymentContainer string              `json:"post_deployment_container"`
	HealthCheck             HealthCheck         `json:"health_check"`
	Limits                  Limits              `json:"limits"`
	Settings                AppSettings         `json:"settings"`
	WebhookSecretEnc        string              `json:"-"`
	ConfigHash              string              `json:"config_hash"`
	CreatedAt               time.Time           `json:"created_at"`
	UpdatedAt               time.Time           `json:"updated_at"`
}

const appCols = `id::text, team_id::text, environment_id::text, destination_id::text, name, description, status, git_repository,
	git_branch, git_commit_sha, source_id::text, private_key_id::text, build_pack, docker_image, docker_image_tag, static_image,
	base_directory, publish_directory, dockerfile, dockerfile_location, dockerfile_target, docker_compose_location,
	docker_compose_raw, docker_compose_domains, install_command, build_command, start_command, watch_paths, ports_exposes,
	ports_mappings, domains, redirect, preview_url_template, custom_labels, custom_docker_run_options, pre_deployment_command,
	pre_deployment_container, post_deployment_command, post_deployment_container, health_check, limits, settings,
	webhook_secret_enc, config_hash, created_at, updated_at`

func scanApp(row pgx.Row) (*Application, error) {
	a := &Application{}
	var domainsJSON, hc, lim, st []byte
	err := row.Scan(&a.ID, &a.TeamID, &a.EnvironmentID, &a.DestinationID, &a.Name, &a.Description, &a.Status, &a.GitRepository,
		&a.GitBranch, &a.GitCommitSHA, &a.SourceID, &a.PrivateKeyID, &a.BuildPack, &a.DockerImage, &a.DockerImageTag, &a.StaticImage,
		&a.BaseDirectory, &a.PublishDirectory, &a.Dockerfile, &a.DockerfileLocation, &a.DockerfileTarget, &a.DockerComposeLocation,
		&a.DockerComposeRaw, &domainsJSON, &a.InstallCommand, &a.BuildCommand, &a.StartCommand, &a.WatchPaths, &a.PortsExposes,
		&a.PortsMappings, &a.Domains, &a.Redirect, &a.PreviewURLTemplate, &a.CustomLabels, &a.CustomDockerRunOptions,
		&a.PreDeploymentCommand, &a.PreDeploymentContainer, &a.PostDeploymentCommand, &a.PostDeploymentContainer, &hc, &lim, &st,
		&a.WebhookSecretEnc, &a.ConfigHash, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	a.HealthCheck, a.Limits, a.Settings = DefaultHealthCheck(), DefaultLimits(), DefaultAppSettings()
	for _, x := range []struct {
		raw []byte
		dst any
	}{{domainsJSON, &a.DockerComposeDomains}, {hc, &a.HealthCheck}, {lim, &a.Limits}, {st, &a.Settings}} {
		if err := json.Unmarshal(x.raw, x.dst); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func appArgs(a *Application) []any {
	cd, _ := json.Marshal(a.DockerComposeDomains)
	hc, _ := json.Marshal(a.HealthCheck)
	lim, _ := json.Marshal(a.Limits)
	st, _ := json.Marshal(a.Settings)
	return []any{a.TeamID, a.EnvironmentID, a.DestinationID, a.Name, a.Description, a.GitRepository, a.GitBranch, a.GitCommitSHA,
		a.SourceID, a.PrivateKeyID, a.BuildPack, a.DockerImage, a.DockerImageTag, a.StaticImage, a.BaseDirectory, a.PublishDirectory,
		a.Dockerfile, a.DockerfileLocation, a.DockerfileTarget, a.DockerComposeLocation, a.DockerComposeRaw, string(cd),
		a.InstallCommand, a.BuildCommand, a.StartCommand, a.WatchPaths, a.PortsExposes, a.PortsMappings, a.Domains, a.Redirect,
		a.PreviewURLTemplate, a.CustomLabels, a.CustomDockerRunOptions, a.PreDeploymentCommand, a.PreDeploymentContainer,
		a.PostDeploymentCommand, a.PostDeploymentContainer, string(hc), string(lim), string(st), a.WebhookSecretEnc}
}

func (s *Store) CreateApplication(ctx context.Context, a *Application) (*Application, error) {
	return scanApp(s.Pool.QueryRow(ctx, `INSERT INTO applications (team_id, environment_id, destination_id, name, description,
		git_repository, git_branch, git_commit_sha, source_id, private_key_id, build_pack, docker_image, docker_image_tag, static_image,
		base_directory, publish_directory, dockerfile, dockerfile_location, dockerfile_target, docker_compose_location,
		docker_compose_raw, docker_compose_domains, install_command, build_command, start_command, watch_paths, ports_exposes,
		ports_mappings, domains, redirect, preview_url_template, custom_labels, custom_docker_run_options, pre_deployment_command,
		pre_deployment_container, post_deployment_command, post_deployment_container, health_check, limits, settings, webhook_secret_enc)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22::jsonb, $23, $24,
		$25, $26, $27, $28, $29, $30, $31, $32, $33, $34, $35, $36, $37, $38::jsonb, $39::jsonb, $40::jsonb, $41)
		RETURNING `+appCols, appArgs(a)...))
}

// UpdateApplication enregistre la configuration (le statut et l'empreinte sont gérés à part).
func (s *Store) UpdateApplication(ctx context.Context, a *Application) error {
	args := append([]any{a.ID}, appArgs(a)[3:]...) // sans team, environnement ni destination
	_, err := s.Pool.Exec(ctx, `UPDATE applications SET name = $2, description = $3, git_repository = $4, git_branch = $5,
		git_commit_sha = $6, source_id = $7, private_key_id = $8, build_pack = $9, docker_image = $10, docker_image_tag = $11,
		static_image = $12, base_directory = $13, publish_directory = $14, dockerfile = $15, dockerfile_location = $16,
		dockerfile_target = $17, docker_compose_location = $18, docker_compose_raw = $19, docker_compose_domains = $20::jsonb,
		install_command = $21, build_command = $22, start_command = $23, watch_paths = $24, ports_exposes = $25,
		ports_mappings = $26, domains = $27, redirect = $28, preview_url_template = $29, custom_labels = $30,
		custom_docker_run_options = $31, pre_deployment_command = $32, pre_deployment_container = $33,
		post_deployment_command = $34, post_deployment_container = $35, health_check = $36::jsonb, limits = $37::jsonb,
		settings = $38::jsonb, webhook_secret_enc = $39, updated_at = now() WHERE id = $1`, args...)
	return err
}

func (s *Store) ApplicationByID(ctx context.Context, teamID, id string) (*Application, error) {
	return scanApp(s.Pool.QueryRow(ctx, `SELECT `+appCols+` FROM applications WHERE team_id = $1 AND id = $2`, teamID, id))
}

// ApplicationByIDAny : sans filtre d'équipe (tâches de fond, webhooks).
func (s *Store) ApplicationByIDAny(ctx context.Context, id string) (*Application, error) {
	return scanApp(s.Pool.QueryRow(ctx, `SELECT `+appCols+` FROM applications WHERE id = $1`, id))
}

func (s *Store) ListApplications(ctx context.Context, teamID, environmentID string) ([]*Application, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+appCols+` FROM applications WHERE team_id = $1 AND ($2 = '' OR environment_id::text = $2)
		ORDER BY lower(name)`, teamID, environmentID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanApp)
}

// ApplicationsByRepo : applications suivant un dépôt (webhooks). repoKeys contient les formes possibles
// (owner/name, URL https, URL ssh) ; la comparaison ignore la casse et le suffixe .git.
func (s *Store) ApplicationsByRepo(ctx context.Context, repoKeys []string) ([]*Application, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+appCols+` FROM applications
		WHERE lower(regexp_replace(git_repository, '\.git$', '')) = ANY($1)`, repoKeys)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanApp)
}

func (s *Store) SetApplicationStatus(ctx context.Context, id, status string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE applications SET status = $2 WHERE id = $1`, id, status)
	return err
}

func (s *Store) SetApplicationConfigHash(ctx context.Context, id, hash string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE applications SET config_hash = $2 WHERE id = $1`, id, hash)
	return err
}

// DeleteApplication supprime l'application et ses données associées (variables, stockages, tags).
func (s *Store) DeleteApplication(ctx context.Context, teamID, id string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		for _, q := range []string{
			`DELETE FROM resource_env_vars WHERE resource_id = $1`,
			`DELETE FROM resource_storages WHERE resource_id = $1`,
			`DELETE FROM resource_tags WHERE resource_id = $1`,
			`DELETE FROM backup_schedules WHERE resource_id = $1`,
			`DELETE FROM scheduled_tasks WHERE resource_id = $1`,
			`DELETE FROM role_assignments WHERE scope_type = 'resource' AND scope_id = $1`,
			`DELETE FROM approval_rules WHERE scope_type = 'resource' AND scope_id = $1`,
		} {
			if _, err := tx.Exec(ctx, q, id); err != nil {
				return err
			}
		}
		_, err := tx.Exec(ctx, `DELETE FROM applications WHERE team_id = $1 AND id = $2`, teamID, id)
		return err
	})
}

// DomainOwners renvoie, pour chaque domaine déjà utilisé par une autre ressource, le nom de cette ressource.
func (s *Store) DomainOwners(ctx context.Context, hosts []string, excludeID string) (map[string]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT host, name FROM (
			SELECT lower(regexp_replace(d, '^https?://([^/:]+).*$', '\1')) AS host, a.name, a.id::text AS rid
			FROM applications a, unnest(a.domains) d
			UNION ALL
			SELECT lower(regexp_replace(d, '^https?://([^/:]+).*$', '\1')), s.name || ' / ' || sa.name, s.id::text
			FROM service_apps sa JOIN services s ON s.id = sa.service_id, unnest(sa.domains) d
		) x WHERE x.rid <> $2 AND x.host = ANY($1)`, hosts, excludeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var host, name string
		if err := rows.Scan(&host, &name); err != nil {
			return nil, err
		}
		out[host] = name
	}
	return out, rows.Err()
}

// ResourceLocation : serveur, projet et environnement d'une ressource (pour les variables partagées et le déploiement).
type ResourceLocation struct {
	ServerID      string
	ProjectID     string
	EnvironmentID string
	Network       string
}

func (s *Store) ResourceLocation(ctx context.Context, resourceID string) (*ResourceLocation, error) {
	l := &ResourceLocation{}
	err := s.Pool.QueryRow(ctx, `SELECT d.server_id::text, e.project_id::text, e.id::text, d.network
		FROM resources r JOIN destinations d ON d.id = r.destination_id JOIN environments e ON e.id = r.environment_id
		WHERE r.id = $1`, resourceID).Scan(&l.ServerID, &l.ProjectID, &l.EnvironmentID, &l.Network)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return l, err
}

// DestinationInTeam renvoie une destination si son serveur appartient à l'équipe.
func (s *Store) DestinationInTeam(ctx context.Context, teamID, id string) (*Destination, *Server, error) {
	var serverID string
	err := s.Pool.QueryRow(ctx, `SELECT d.server_id::text FROM destinations d JOIN servers s ON s.id = d.server_id
		WHERE s.team_id = $1 AND d.id = $2`, teamID, id).Scan(&serverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	} else if err != nil {
		return nil, nil, err
	}
	d, err := s.DestinationByID(ctx, serverID, id)
	if err != nil {
		return nil, nil, err
	}
	sv, err := s.ServerByID(ctx, teamID, serverID)
	return d, sv, err
}
