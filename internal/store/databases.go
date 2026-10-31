package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// InitScript : script exécuté au premier démarrage de la base (dossier docker-entrypoint-initdb.d).
type InitScript struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

// DatabaseSettings : options d'une base, dont celles propres à un type.
type DatabaseSettings struct {
	HealthCheckEnabled     bool   `json:"health_check_enabled"`
	StopGracePeriod        int    `json:"stop_grace_period"`
	PostgresInitdbArgs     string `json:"postgres_initdb_args"`      // POSTGRES_INITDB_ARGS
	PostgresHostAuthMethod string `json:"postgres_host_auth_method"` // POSTGRES_HOST_AUTH_METHOD
	LogDrainEnabled        bool   `json:"log_drain_enabled"`         // logs envoyés au log drain du serveur
	// TLS : certificats signés par l'autorité du serveur, régénérés à chaque démarrage.
	EnableSSL bool   `json:"enable_ssl"`
	SSLMode   string `json:"ssl_mode"`
}

func DefaultDatabaseSettings() DatabaseSettings {
	return DatabaseSettings{HealthCheckEnabled: true, StopGracePeriod: 30}
}

type Database struct {
	ID                     string           `json:"id"`
	TeamID                 string           `json:"team_id"`
	EnvironmentID          string           `json:"environment_id"`
	DestinationID          string           `json:"destination_id"`
	Type                   string           `json:"type"`
	Name                   string           `json:"name"`
	Description            string           `json:"description"`
	Image                  string           `json:"image"`
	Status                 string           `json:"status"`
	Username               string           `json:"username"`
	PasswordEnc            string           `json:"-"`
	RootPasswordEnc        string           `json:"-"`
	DatabaseName           string           `json:"database_name"`
	CustomConfig           string           `json:"custom_config"`
	InitScripts            []InitScript     `json:"init_scripts"`
	IsPublic               bool             `json:"is_public"`
	PublicPort             *int             `json:"public_port"`
	PortsMappings          string           `json:"ports_mappings"`
	CustomDockerRunOptions string           `json:"custom_docker_run_options"`
	Limits                 Limits           `json:"limits"`
	Settings               DatabaseSettings `json:"settings"`
	StartedAt              *time.Time       `json:"started_at"`
	CreatedAt              time.Time        `json:"created_at"`
	UpdatedAt              time.Time        `json:"updated_at"`
}

const dbCols = `id::text, team_id::text, environment_id::text, destination_id::text, type, name, description, image, status,
	username, password_enc, root_password_enc, database_name, custom_config, init_scripts, is_public, public_port, ports_mappings,
	custom_docker_run_options, limits, settings, started_at, created_at, updated_at`

func scanDatabase(row pgx.Row) (*Database, error) {
	d := &Database{}
	var scripts, lim, st []byte
	err := row.Scan(&d.ID, &d.TeamID, &d.EnvironmentID, &d.DestinationID, &d.Type, &d.Name, &d.Description, &d.Image, &d.Status,
		&d.Username, &d.PasswordEnc, &d.RootPasswordEnc, &d.DatabaseName, &d.CustomConfig, &scripts, &d.IsPublic, &d.PublicPort,
		&d.PortsMappings, &d.CustomDockerRunOptions, &lim, &st, &d.StartedAt, &d.CreatedAt, &d.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	d.Limits, d.Settings = DefaultLimits(), DefaultDatabaseSettings()
	for _, x := range []struct {
		raw []byte
		dst any
	}{{scripts, &d.InitScripts}, {lim, &d.Limits}, {st, &d.Settings}} {
		if err := json.Unmarshal(x.raw, x.dst); err != nil {
			return nil, err
		}
	}
	if d.InitScripts == nil {
		d.InitScripts = []InitScript{}
	}
	return d, nil
}

func dbArgs(d *Database) []any {
	scripts := d.InitScripts
	if scripts == nil {
		scripts = []InitScript{}
	}
	sc, _ := json.Marshal(scripts)
	lim, _ := json.Marshal(d.Limits)
	st, _ := json.Marshal(d.Settings)
	return []any{d.TeamID, d.EnvironmentID, d.DestinationID, d.Type, d.Name, d.Description, d.Image, d.Username, d.PasswordEnc,
		d.RootPasswordEnc, d.DatabaseName, d.CustomConfig, string(sc), d.IsPublic, d.PublicPort, d.PortsMappings,
		d.CustomDockerRunOptions, string(lim), string(st)}
}

// CreateDatabase crée la base et ses stockages par défaut (volumes de données, dont le nom dépend de
// l'identifiant attribué) dans une même transaction.
func (s *Store) CreateDatabase(ctx context.Context, d *Database, storages func(id string) []*Storage) (*Database, error) {
	var created *Database
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		created, err = scanDatabase(tx.QueryRow(ctx, `INSERT INTO databases (team_id, environment_id, destination_id, type, name,
			description, image, username, password_enc, root_password_enc, database_name, custom_config, init_scripts, is_public,
			public_port, ports_mappings, custom_docker_run_options, limits, settings)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14, $15, $16, $17, $18::jsonb, $19::jsonb)
			RETURNING `+dbCols, dbArgs(d)...))
		if err != nil {
			return err
		}
		for _, st := range storages(created.ID) {
			if _, err := tx.Exec(ctx, `INSERT INTO resource_storages (resource_type, resource_id, kind, name, host_path, mount_path, content)
				VALUES ('database', $1, $2, $3, $4, $5, $6)`, created.ID, st.Kind, st.Name, st.HostPath, st.MountPath, st.Content); err != nil {
				return err
			}
		}
		return nil
	})
	return created, err
}

// UpdateDatabase enregistre la configuration (le statut est géré à part).
func (s *Store) UpdateDatabase(ctx context.Context, d *Database) error {
	args := append([]any{d.ID}, dbArgs(d)[4:]...) // sans équipe, environnement, destination ni type
	_, err := s.Pool.Exec(ctx, `UPDATE databases SET name = $2, description = $3, image = $4, username = $5, password_enc = $6,
		root_password_enc = $7, database_name = $8, custom_config = $9, init_scripts = $10::jsonb, is_public = $11, public_port = $12,
		ports_mappings = $13, custom_docker_run_options = $14, limits = $15::jsonb, settings = $16::jsonb, updated_at = now()
		WHERE id = $1`, args...)
	return err
}

func (s *Store) DatabaseByID(ctx context.Context, teamID, id string) (*Database, error) {
	return scanDatabase(s.Pool.QueryRow(ctx, `SELECT `+dbCols+` FROM databases WHERE team_id = $1 AND id = $2`, teamID, id))
}

// DatabaseByIDAny : sans filtre d'équipe (tâches de fond).
func (s *Store) DatabaseByIDAny(ctx context.Context, id string) (*Database, error) {
	return scanDatabase(s.Pool.QueryRow(ctx, `SELECT `+dbCols+` FROM databases WHERE id = $1`, id))
}

func (s *Store) ListDatabases(ctx context.Context, teamID, environmentID string) ([]*Database, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+dbCols+` FROM databases WHERE team_id = $1 AND ($2 = '' OR environment_id::text = $2)
		ORDER BY lower(name)`, teamID, environmentID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanDatabase)
}

// SetDatabaseStatus met à jour le statut ; started marque aussi l'heure du démarrage.
func (s *Store) SetDatabaseStatus(ctx context.Context, id, status string, started bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE databases SET status = $2, started_at = CASE WHEN $3 THEN now() ELSE started_at END
		WHERE id = $1`, id, status, started)
	if err == nil {
		s.emitResource(ctx, "databases", id)
	}
	return err
}

// DeleteDatabase supprime la base et ses données associées (variables, stockages, tags, droits, règles).
func (s *Store) DeleteDatabase(ctx context.Context, teamID, id string) error {
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
		_, err := tx.Exec(ctx, `DELETE FROM databases WHERE team_id = $1 AND id = $2`, teamID, id)
		return err
	})
}

// HostPortsInUse renvoie les ports publiés sur l'hôte par les ressources d'un serveur (hors excludeID) :
// port public des bases et ports_mappings des applications et des bases, avec le nom de la ressource.
func (s *Store) HostPortsInUse(ctx context.Context, serverID, excludeID string) (publicPorts map[int]string, mappings map[string]string, err error) {
	rows, err := s.Pool.Query(ctx, `SELECT x.name, x.public_port, x.ports_mappings FROM (
			SELECT id, name, destination_id, CASE WHEN is_public THEN public_port END AS public_port, ports_mappings FROM databases
			UNION ALL SELECT id, name, destination_id, NULL, ports_mappings FROM applications
		) x JOIN destinations d ON d.id = x.destination_id WHERE d.server_id = $1 AND x.id::text <> $2`, serverID, excludeID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	publicPorts, mappings = map[int]string{}, map[string]string{}
	for rows.Next() {
		var name, m string
		var port *int
		if err := rows.Scan(&name, &port, &m); err != nil {
			return nil, nil, err
		}
		if port != nil {
			publicPorts[*port] = name
		}
		if m != "" {
			mappings[name] = m
		}
	}
	return publicPorts, mappings, rows.Err()
}

// DatabaseTasks : dernières actions (tâches serveur) d'une base.
func (s *Store) DatabaseTasks(ctx context.Context, databaseID string, limit int) ([]*ServerTask, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+taskCols+` FROM server_tasks WHERE params ? 'database_id' AND params->>'database_id' = $1
		ORDER BY created_at DESC LIMIT $2`, databaseID, limit)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanTask)
}

// HasActiveDatabaseTask indique si une action est en file ou en cours sur la base.
func (s *Store) HasActiveDatabaseTask(ctx context.Context, databaseID string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM server_tasks WHERE params ? 'database_id'
		AND params->>'database_id' = $1 AND status IN ('queued', 'running'))`, databaseID).Scan(&ok)
	return ok, err
}
