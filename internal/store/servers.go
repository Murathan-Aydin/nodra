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
	scopeParents[rbac.ScopeServer] = func(ctx context.Context, s *Store, id string) (rbac.Scope, error) {
		var team string
		err := s.Pool.QueryRow(ctx, `SELECT team_id::text FROM servers WHERE id = $1`, id).Scan(&team)
		if errors.Is(err, pgx.ErrNoRows) {
			return rbac.Scope{}, ErrNotFound
		}
		return rbac.Scope{Type: rbac.ScopeTeam, ID: team}, err
	}
}

// --- Clés privées ---

type PrivateKey struct {
	ID            string    `json:"id"`
	TeamID        string    `json:"team_id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	PrivateKeyEnc string    `json:"-"`
	PublicKey     string    `json:"public_key"`
	Fingerprint   string    `json:"fingerprint"`
	CreatedAt     time.Time `json:"created_at"`
}

const keyCols = `id::text, team_id::text, name, description, private_key_enc, public_key, fingerprint, created_at`

func scanKey(row pgx.Row) (*PrivateKey, error) {
	k := &PrivateKey{}
	err := row.Scan(&k.ID, &k.TeamID, &k.Name, &k.Description, &k.PrivateKeyEnc, &k.PublicKey, &k.Fingerprint, &k.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return k, err
}

func (s *Store) CreatePrivateKey(ctx context.Context, k *PrivateKey) (*PrivateKey, error) {
	r, err := scanKey(s.Pool.QueryRow(ctx, `INSERT INTO private_keys (team_id, name, description, private_key_enc, public_key, fingerprint)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+keyCols, k.TeamID, k.Name, k.Description, k.PrivateKeyEnc, k.PublicKey, k.Fingerprint))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return r, err
}

func (s *Store) ListPrivateKeys(ctx context.Context, teamID string) ([]*PrivateKey, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+keyCols+` FROM private_keys WHERE team_id = $1 ORDER BY name`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanKey)
}

func (s *Store) PrivateKeyByID(ctx context.Context, teamID, id string) (*PrivateKey, error) {
	return scanKey(s.Pool.QueryRow(ctx, `SELECT `+keyCols+` FROM private_keys WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) UpdatePrivateKey(ctx context.Context, teamID, id, name, desc string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE private_keys SET name = $3, description = $4, updated_at = now() WHERE team_id = $1 AND id = $2`,
		teamID, id, name, desc)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) DeletePrivateKey(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM private_keys WHERE team_id = $1 AND id = $2`, teamID, id)
	if isForeignKeyViolation(err) {
		return ErrInUse
	}
	return err
}

// --- Serveurs ---

// ServerSettings : réglages d'un serveur (stockés en JSON, valeurs par défaut appliquées à la lecture).
type ServerSettings struct {
	ConcurrentBuilds       int    `json:"concurrent_builds"`
	DeploymentTimeout      int    `json:"deployment_timeout"` // secondes
	IsBuildServer          bool   `json:"is_build_server"`
	DockerCleanupFrequency string `json:"docker_cleanup_frequency"` // expression cron
	DockerCleanupThreshold int    `json:"docker_cleanup_threshold"` // % disque déclenchant un nettoyage
	DeleteUnusedVolumes    bool   `json:"delete_unused_volumes"`
	DeleteUnusedNetworks   bool   `json:"delete_unused_networks"`
	MetricsEnabled         bool   `json:"metrics_enabled"`
	MetricsRetentionDays   int    `json:"metrics_retention_days"`
	Timezone               string `json:"timezone"`
	WildcardDomain         string `json:"wildcard_domain"`
	ConnectionTimeout      int    `json:"connection_timeout"` // secondes
	CloudflareTunnel       bool   `json:"cloudflare_tunnel"`
}

func DefaultServerSettings() ServerSettings {
	return ServerSettings{ConcurrentBuilds: 2, DeploymentTimeout: 3600, DockerCleanupFrequency: "0 0 * * *",
		DockerCleanupThreshold: 80, MetricsEnabled: true, MetricsRetentionDays: 7, Timezone: "UTC", ConnectionTimeout: 10}
}

type Server struct {
	ID              string         `json:"id"`
	TeamID          string         `json:"team_id"`
	Name            string         `json:"name"`
	Description     string         `json:"description"`
	IP              string         `json:"ip"`
	Port            int            `json:"port"`
	User            string         `json:"user"`
	PrivateKeyID    string         `json:"private_key_id"`
	HostKey         string         `json:"host_key"`
	IsReachable     bool           `json:"is_reachable"`
	IsUsable        bool           `json:"is_usable"`
	OSID            string         `json:"os_id"`
	OSVersion       string         `json:"os_version"`
	DockerVersion   string         `json:"docker_version"`
	ComposeVersion  string         `json:"compose_version"`
	ValidationError string         `json:"validation_error"`
	ValidatedAt     *time.Time     `json:"validated_at"`
	LastSeenAt      *time.Time     `json:"last_seen_at"`
	LastCleanupAt   *time.Time     `json:"last_cleanup_at"`
	Settings        ServerSettings `json:"settings"`
	Proxy           ProxyInfo      `json:"proxy"`
	CreatedAt       time.Time      `json:"created_at"`
}

// ProxyInfo : proxy inverse du serveur (Traefik, Caddy ou aucun).
type ProxyInfo struct {
	Type      string `json:"type"`
	Status    string `json:"status"`
	Compose   string `json:"-"` // configuration personnalisée, vide = générée
	AcmeEmail string `json:"acme_email"`
}

const serverCols = `id::text, team_id::text, name, description, ip, port, ssh_user, private_key_id::text, host_key,
	is_reachable, is_usable, os_id, os_version, docker_version, compose_version, validation_error, validated_at,
	last_seen_at, last_cleanup_at, settings, proxy_type, proxy_status, proxy_compose, proxy_acme_email, created_at`

func scanServer(row pgx.Row) (*Server, error) {
	s := &Server{}
	var raw []byte
	err := row.Scan(&s.ID, &s.TeamID, &s.Name, &s.Description, &s.IP, &s.Port, &s.User, &s.PrivateKeyID, &s.HostKey,
		&s.IsReachable, &s.IsUsable, &s.OSID, &s.OSVersion, &s.DockerVersion, &s.ComposeVersion, &s.ValidationError,
		&s.ValidatedAt, &s.LastSeenAt, &s.LastCleanupAt, &raw, &s.Proxy.Type, &s.Proxy.Status, &s.Proxy.Compose,
		&s.Proxy.AcmeEmail, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	s.Settings = DefaultServerSettings()
	return s, json.Unmarshal(raw, &s.Settings)
}

func (s *Store) CreateServer(ctx context.Context, sv *Server) (*Server, error) {
	settings, _ := json.Marshal(sv.Settings)
	if sv.Proxy.Type == "" {
		sv.Proxy.Type = "traefik"
	}
	r, err := scanServer(s.Pool.QueryRow(ctx, `INSERT INTO servers (team_id, name, description, ip, port, ssh_user, private_key_id, settings, proxy_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9) RETURNING `+serverCols,
		sv.TeamID, sv.Name, sv.Description, sv.IP, sv.Port, sv.User, sv.PrivateKeyID, string(settings), sv.Proxy.Type))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return r, err
}

func (s *Store) ListServers(ctx context.Context, teamID string) ([]*Server, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+serverCols+` FROM servers WHERE team_id = $1 ORDER BY name`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanServer)
}

func (s *Store) ServerByID(ctx context.Context, teamID, id string) (*Server, error) {
	return scanServer(s.Pool.QueryRow(ctx, `SELECT `+serverCols+` FROM servers WHERE team_id = $1 AND id = $2`, teamID, id))
}

// ServerByIDAny : sans filtre d'équipe (tâches de fond uniquement).
func (s *Store) ServerByIDAny(ctx context.Context, id string) (*Server, error) {
	return scanServer(s.Pool.QueryRow(ctx, `SELECT `+serverCols+` FROM servers WHERE id = $1`, id))
}

// UsableServers : serveurs validés (tâches de fond : métriques, nettoyage).
func (s *Store) UsableServers(ctx context.Context) ([]*Server, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+serverCols+` FROM servers WHERE is_usable ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanServer)
}

// UpdateServer met à jour la configuration. Changer d'adresse, de port ou de clé invalide la clé d'hôte et la validation.
func (s *Store) UpdateServer(ctx context.Context, sv *Server) error {
	settings, _ := json.Marshal(sv.Settings)
	_, err := s.Pool.Exec(ctx, `UPDATE servers SET name = $3, description = $4, settings = $8::jsonb,
			host_key = CASE WHEN (ip, port, private_key_id) IS DISTINCT FROM ($5, $6, $7::uuid) THEN '' ELSE host_key END,
			is_usable = CASE WHEN (ip, port, ssh_user, private_key_id) IS DISTINCT FROM ($5, $6, $9, $7::uuid) THEN false ELSE is_usable END,
			ip = $5, port = $6, private_key_id = $7, ssh_user = $9, updated_at = now()
		WHERE team_id = $1 AND id = $2`,
		sv.TeamID, sv.ID, sv.Name, sv.Description, sv.IP, sv.Port, sv.PrivateKeyID, string(settings), sv.User)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) DeleteServer(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM servers WHERE team_id = $1 AND id = $2`, teamID, id)
	if isForeignKeyViolation(err) {
		return ErrInUse
	}
	return err
}

func (s *Store) SetServerHostKey(ctx context.Context, id, hostKey string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE servers SET host_key = $2, updated_at = now() WHERE id = $1`, id, hostKey)
	return err
}

type ServerValidation struct {
	Reachable, Usable                              bool
	OSID, OSVersion, DockerVersion, ComposeVersion string
	Error                                          string
}

func (s *Store) SetServerValidation(ctx context.Context, id string, v ServerValidation) error {
	_, err := s.Pool.Exec(ctx, `UPDATE servers SET is_reachable = $2, is_usable = $3, os_id = $4, os_version = $5,
		docker_version = $6, compose_version = $7, validation_error = $8, validated_at = now(),
		last_seen_at = CASE WHEN $2 THEN now() ELSE last_seen_at END, updated_at = now() WHERE id = $1`,
		id, v.Reachable, v.Usable, v.OSID, v.OSVersion, v.DockerVersion, v.ComposeVersion, v.Error)
	return err
}

func (s *Store) SetServerReachable(ctx context.Context, id string, reachable bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE servers SET is_reachable = $2, last_seen_at = CASE WHEN $2 THEN now() ELSE last_seen_at END
		WHERE id = $1`, id, reachable)
	return err
}

func (s *Store) SetServerCleanedUp(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE servers SET last_cleanup_at = now() WHERE id = $1`, id)
	return err
}

// --- Destinations ---

type Destination struct {
	ID        string    `json:"id"`
	ServerID  string    `json:"server_id"`
	Name      string    `json:"name"`
	Network   string    `json:"network"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}

func scanDestination(row pgx.Row) (*Destination, error) {
	d := &Destination{}
	err := row.Scan(&d.ID, &d.ServerID, &d.Name, &d.Network, &d.IsDefault, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

const destCols = `id::text, server_id::text, name, network, is_default, created_at`

func (s *Store) ListDestinations(ctx context.Context, serverID string) ([]*Destination, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+destCols+` FROM destinations WHERE server_id = $1 ORDER BY is_default DESC, name`, serverID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanDestination)
}

func (s *Store) DestinationByID(ctx context.Context, serverID, id string) (*Destination, error) {
	return scanDestination(s.Pool.QueryRow(ctx, `SELECT `+destCols+` FROM destinations WHERE server_id = $1 AND id = $2`, serverID, id))
}

// UpsertDestination crée la destination si elle n'existe pas (idempotent).
func (s *Store) UpsertDestination(ctx context.Context, serverID, name, network string, isDefault bool) (*Destination, error) {
	return scanDestination(s.Pool.QueryRow(ctx, `INSERT INTO destinations (server_id, name, network, is_default)
		VALUES ($1, $2, $3, $4) ON CONFLICT (server_id, network) DO UPDATE SET name = destinations.name
		RETURNING `+destCols, serverID, name, network, isDefault))
}

func (s *Store) DeleteDestination(ctx context.Context, serverID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM destinations WHERE server_id = $1 AND id = $2`, serverID, id)
	if isForeignKeyViolation(err) {
		return ErrInUse
	}
	return err
}

// --- Tâches ---

type ServerTask struct {
	ID         string          `json:"id"`
	TeamID     string          `json:"team_id"`
	ServerID   string          `json:"server_id"`
	Kind       string          `json:"kind"`
	Params     json.RawMessage `json:"params"`
	Status     string          `json:"status"`
	Output     string          `json:"output"`
	Error      string          `json:"error"`
	CreatedBy  *string         `json:"created_by"`
	CreatedAt  time.Time       `json:"created_at"`
	StartedAt  *time.Time      `json:"started_at"`
	FinishedAt *time.Time      `json:"finished_at"`
}

const taskCols = `id::text, team_id::text, server_id::text, kind, params, status, output, error, created_by::text,
	created_at, started_at, finished_at`

func scanTask(row pgx.Row) (*ServerTask, error) {
	t := &ServerTask{}
	err := row.Scan(&t.ID, &t.TeamID, &t.ServerID, &t.Kind, &t.Params, &t.Status, &t.Output, &t.Error, &t.CreatedBy,
		&t.CreatedAt, &t.StartedAt, &t.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// CreateServerTask insère la tâche ; tx permet d'enfiler le job River dans la même transaction.
func (s *Store) CreateServerTask(ctx context.Context, tx pgx.Tx, t *ServerTask) (*ServerTask, error) {
	params := t.Params
	if len(params) == 0 {
		params = json.RawMessage(`{}`)
	}
	return scanTask(tx.QueryRow(ctx, `INSERT INTO server_tasks (team_id, server_id, kind, params, created_by)
		VALUES ($1, $2, $3, $4::jsonb, $5) RETURNING `+taskCols, t.TeamID, t.ServerID, t.Kind, string(params), t.CreatedBy))
}

func (s *Store) ServerTaskByID(ctx context.Context, id string) (*ServerTask, error) {
	return scanTask(s.Pool.QueryRow(ctx, `SELECT `+taskCols+` FROM server_tasks WHERE id = $1`, id))
}

func (s *Store) ListServerTasks(ctx context.Context, serverID string, limit int) ([]*ServerTask, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+taskCols+` FROM server_tasks WHERE server_id = $1 ORDER BY created_at DESC LIMIT $2`, serverID, limit)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanTask)
}

// StartServerTask passe la tâche en cours ; false si elle n'était plus en file (annulée).
func (s *Store) StartServerTask(ctx context.Context, id string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE server_tasks SET status = 'running', started_at = now() WHERE id = $1 AND status = 'queued'`, id)
	return tag.RowsAffected() == 1, err
}

func (s *Store) AppendServerTaskOutput(ctx context.Context, id, chunk string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE server_tasks SET output = right(output || $2, 1000000) WHERE id = $1`, id, chunk)
	return err
}

func (s *Store) FinishServerTask(ctx context.Context, id, status, errMsg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE server_tasks SET status = $2, error = $3, finished_at = now()
		WHERE id = $1 AND status IN ('queued', 'running')`, id, status, errMsg)
	return err
}

// --- Métriques ---

type ServerMetric struct {
	TS         time.Time `json:"ts"`
	CPUPercent float32   `json:"cpu_percent"`
	MemUsed    int64     `json:"mem_used"`
	MemTotal   int64     `json:"mem_total"`
	DiskUsed   int64     `json:"disk_used"`
	DiskTotal  int64     `json:"disk_total"`
	Load1      float32   `json:"load1"`
}

func (s *Store) InsertServerMetric(ctx context.Context, serverID string, m ServerMetric) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO server_metrics (server_id, ts, cpu_percent, mem_used, mem_total, disk_used, disk_total, load1)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT DO NOTHING`,
		serverID, m.TS, m.CPUPercent, m.MemUsed, m.MemTotal, m.DiskUsed, m.DiskTotal, m.Load1)
	return err
}

func (s *Store) ServerMetrics(ctx context.Context, serverID string, since time.Time) ([]ServerMetric, error) {
	rows, err := s.Pool.Query(ctx, `SELECT ts, cpu_percent, mem_used, mem_total, disk_used, disk_total, load1
		FROM server_metrics WHERE server_id = $1 AND ts >= $2 ORDER BY ts LIMIT 20000`, serverID, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (ServerMetric, error) {
		var m ServerMetric
		err := row.Scan(&m.TS, &m.CPUPercent, &m.MemUsed, &m.MemTotal, &m.DiskUsed, &m.DiskTotal, &m.Load1)
		return m, err
	})
}

// PurgeServerMetrics supprime les points plus anciens que la rétention de chaque serveur.
func (s *Store) PurgeServerMetrics(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM server_metrics m USING servers s WHERE m.server_id = s.id
		AND m.ts < now() - make_interval(days => coalesce((s.settings->>'metrics_retention_days')::int, 7))`)
	return err
}

// HasActiveServerTask indique si une tâche du type donné est en file ou en cours sur le serveur.
func (s *Store) HasActiveServerTask(ctx context.Context, serverID, kind string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM server_tasks WHERE server_id = $1 AND kind = $2
		AND status IN ('queued', 'running'))`, serverID, kind).Scan(&ok)
	return ok, err
}

// FailStaleServerTasks marque en échec les tâches bloquées (processus arrêté pendant l'exécution).
func (s *Store) FailStaleServerTasks(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE server_tasks SET status = 'failed', error = 'tâche interrompue', finished_at = now()
		WHERE (status = 'running' AND started_at < now() - interval '3 hours')
		   OR (status = 'queued' AND created_at < now() - interval '1 day')`)
	return err
}

// CancelServerTask annule une tâche encore en file.
func (s *Store) CancelServerTask(ctx context.Context, id string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE server_tasks SET status = 'cancelled', finished_at = now() WHERE id = $1 AND status = 'queued'`, id)
	return tag.RowsAffected() == 1, err
}

// ResetServerHostKey oublie la clé d'hôte (serveur réinstallé) : une nouvelle validation est nécessaire.
func (s *Store) ResetServerHostKey(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE servers SET host_key = '', is_usable = false, updated_at = now() WHERE team_id = $1 AND id = $2`, teamID, id)
	return err
}

// HasActiveTaskWithParam indique si une tâche en file ou en cours porte params[key] = value.
func (s *Store) HasActiveTaskWithParam(ctx context.Context, key, value string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM server_tasks WHERE params->>$1 = $2 AND status IN ('queued', 'running'))`,
		key, value).Scan(&ok)
	return ok, err
}

// TasksWithParam : tâches portant params[key] = value (historique d'une sauvegarde…).
func (s *Store) TasksWithParam(ctx context.Context, key, value string, limit int) ([]*ServerTask, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+taskCols+` FROM server_tasks WHERE params->>$1 = $2 ORDER BY created_at DESC LIMIT $3`,
		key, value, limit)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanTask)
}
