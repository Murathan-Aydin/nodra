package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Service struct {
	ID               string    `json:"id"`
	TeamID           string    `json:"team_id"`
	EnvironmentID    string    `json:"environment_id"`
	DestinationID    string    `json:"destination_id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Template         string    `json:"template"`
	DockerComposeRaw string    `json:"docker_compose_raw"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	LogDrainEnabled  bool      `json:"log_drain_enabled"`
}

const serviceCols = `id::text, team_id::text, environment_id::text, destination_id::text, name, description, template,
	docker_compose_raw, status, log_drain_enabled, created_at, updated_at`

func scanService(row pgx.Row) (*Service, error) {
	s := &Service{}
	err := row.Scan(&s.ID, &s.TeamID, &s.EnvironmentID, &s.DestinationID, &s.Name, &s.Description, &s.Template,
		&s.DockerComposeRaw, &s.Status, &s.LogDrainEnabled, &s.CreatedAt, &s.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

type ServiceApp struct {
	ID                string   `json:"id"`
	ServiceID         string   `json:"service_id"`
	Name              string   `json:"name"`
	Image             string   `json:"image"`
	Domains           []string `json:"domains"`
	Port              string   `json:"port"`
	URLVar            string   `json:"url_var"`
	ExcludeFromStatus bool     `json:"exclude_from_status"`
}

const serviceAppCols = `id::text, service_id::text, name, image, domains, port, url_var, exclude_from_status`

func scanServiceApp(row pgx.Row) (*ServiceApp, error) {
	a := &ServiceApp{}
	err := row.Scan(&a.ID, &a.ServiceID, &a.Name, &a.Image, &a.Domains, &a.Port, &a.URLVar, &a.ExcludeFromStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// CreateService crée le service, ses sous-services et ses variables générées dans une transaction.
func (s *Store) CreateService(ctx context.Context, svc *Service, apps []*ServiceApp, vars []*EnvVar) (*Service, error) {
	var out *Service
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var err error
		if out, err = scanService(tx.QueryRow(ctx, `INSERT INTO services (team_id, environment_id, destination_id, name, description,
			template, docker_compose_raw) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+serviceCols,
			svc.TeamID, svc.EnvironmentID, svc.DestinationID, svc.Name, svc.Description, svc.Template, svc.DockerComposeRaw)); err != nil {
			return err
		}
		return syncServiceTx(ctx, tx, out.ID, apps, vars)
	})
	return out, err
}

// syncServiceTx ajoute/supprime les sous-services (les domaines des existants sont conservés)
// et ajoute les variables manquantes (sans écraser les valeurs existantes).
func syncServiceTx(ctx context.Context, tx pgx.Tx, serviceID string, apps []*ServiceApp, vars []*EnvVar) error {
	names := make([]string, len(apps))
	for i, a := range apps {
		names[i] = a.Name
		if _, err := tx.Exec(ctx, `INSERT INTO service_apps (service_id, name, image, domains, port, url_var, exclude_from_status)
			VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (service_id, name) DO UPDATE SET image = EXCLUDED.image,
			url_var = EXCLUDED.url_var, port = CASE WHEN service_apps.port = '' THEN EXCLUDED.port ELSE service_apps.port END`,
			serviceID, a.Name, a.Image, a.Domains, a.Port, a.URLVar, a.ExcludeFromStatus); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM service_apps WHERE service_id = $1 AND NOT (name = ANY($2))`, serviceID, names); err != nil {
		return err
	}
	for _, v := range vars {
		if _, err := tx.Exec(ctx, `INSERT INTO resource_env_vars (resource_type, resource_id, key, value_enc, is_buildtime, is_runtime,
			is_secret, comment) VALUES ('service', $1, $2, $3, false, true, false, $4) ON CONFLICT DO NOTHING`,
			serviceID, v.Key, v.ValueEnc, v.Comment); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) UpdateService(ctx context.Context, svc *Service, apps []*ServiceApp, vars []*EnvVar) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE services SET name = $2, description = $3, docker_compose_raw = $4, updated_at = now()
			WHERE id = $1`, svc.ID, svc.Name, svc.Description, svc.DockerComposeRaw); err != nil {
			return err
		}
		if apps == nil {
			return nil
		}
		return syncServiceTx(ctx, tx, svc.ID, apps, vars)
	})
}

func (s *Store) ServiceByID(ctx context.Context, teamID, id string) (*Service, error) {
	return scanService(s.Pool.QueryRow(ctx, `SELECT `+serviceCols+` FROM services WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) ServiceByIDAny(ctx context.Context, id string) (*Service, error) {
	return scanService(s.Pool.QueryRow(ctx, `SELECT `+serviceCols+` FROM services WHERE id = $1`, id))
}

func (s *Store) ListServices(ctx context.Context, teamID, environmentID string) ([]*Service, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+serviceCols+` FROM services WHERE team_id = $1 AND ($2 = '' OR environment_id::text = $2)
		ORDER BY lower(name)`, teamID, environmentID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanService)
}

func (s *Store) ServiceApps(ctx context.Context, serviceID string) ([]*ServiceApp, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+serviceAppCols+` FROM service_apps WHERE service_id = $1 ORDER BY name`, serviceID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanServiceApp)
}

func (s *Store) UpdateServiceApp(ctx context.Context, serviceID, name string, domains []string, port string, exclude bool) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE service_apps SET domains = $3, port = $4, exclude_from_status = $5
		WHERE service_id = $1 AND name = $2`, serviceID, name, domains, port, exclude)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) SetServiceStatus(ctx context.Context, id, status string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE services SET status = $2 WHERE id = $1`, id, status)
	if err == nil {
		s.emitResource(ctx, "services", id)
	}
	return err
}

// DeleteService supprime le service et ses données associées.
func (s *Store) DeleteService(ctx context.Context, teamID, id string) error {
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
		_, err := tx.Exec(ctx, `DELETE FROM services WHERE team_id = $1 AND id = $2`, teamID, id)
		return err
	})
}

// ServiceTasks : tâches serveur d'un service (actions start/stop…).
func (s *Store) ServiceTasks(ctx context.Context, serviceID string, limit int) ([]*ServerTask, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+taskCols+` FROM server_tasks WHERE params->>'service_id' = $1
		ORDER BY created_at DESC LIMIT $2`, serviceID, limit)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanTask)
}

func (s *Store) HasActiveServiceTask(ctx context.Context, serviceID string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM server_tasks WHERE params->>'service_id' = $1
		AND status IN ('queued', 'running'))`, serviceID).Scan(&ok)
	return ok, err
}
