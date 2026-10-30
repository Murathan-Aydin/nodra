package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type ScheduledTask struct {
	ID         string     `json:"id"`
	TeamID     string     `json:"team_id"`
	ResourceID string     `json:"resource_id"`
	Name       string     `json:"name"`
	Command    string     `json:"command"`
	Frequency  string     `json:"frequency"`
	Container  string     `json:"container"`
	Timeout    int        `json:"timeout"`
	Enabled    bool       `json:"enabled"`
	LastRunAt  *time.Time `json:"last_run_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

const schedTaskCols = `id::text, team_id::text, resource_id::text, name, command, frequency, container, timeout, enabled, last_run_at, created_at`

func scanSchedTask(row pgx.Row) (*ScheduledTask, error) {
	t := &ScheduledTask{}
	err := row.Scan(&t.ID, &t.TeamID, &t.ResourceID, &t.Name, &t.Command, &t.Frequency, &t.Container, &t.Timeout, &t.Enabled,
		&t.LastRunAt, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

func (s *Store) CreateScheduledTask(ctx context.Context, t *ScheduledTask) (*ScheduledTask, error) {
	return scanSchedTask(s.Pool.QueryRow(ctx, `INSERT INTO scheduled_tasks (team_id, resource_id, name, command, frequency, container,
		timeout, enabled) VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+schedTaskCols,
		t.TeamID, t.ResourceID, t.Name, t.Command, t.Frequency, t.Container, t.Timeout, t.Enabled))
}

func (s *Store) UpdateScheduledTask(ctx context.Context, t *ScheduledTask) error {
	_, err := s.Pool.Exec(ctx, `UPDATE scheduled_tasks SET name = $2, command = $3, frequency = $4, container = $5, timeout = $6,
		enabled = $7, updated_at = now() WHERE id = $1`, t.ID, t.Name, t.Command, t.Frequency, t.Container, t.Timeout, t.Enabled)
	return err
}

func (s *Store) ScheduledTaskByID(ctx context.Context, teamID, id string) (*ScheduledTask, error) {
	return scanSchedTask(s.Pool.QueryRow(ctx, `SELECT `+schedTaskCols+` FROM scheduled_tasks WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) ScheduledTaskByIDAny(ctx context.Context, id string) (*ScheduledTask, error) {
	return scanSchedTask(s.Pool.QueryRow(ctx, `SELECT `+schedTaskCols+` FROM scheduled_tasks WHERE id = $1`, id))
}

func (s *Store) ListScheduledTasks(ctx context.Context, resourceID string) ([]*ScheduledTask, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+schedTaskCols+` FROM scheduled_tasks WHERE resource_id = $1 ORDER BY lower(name)`, resourceID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSchedTask)
}

// EnabledScheduledTasks : tâches actives avec le fuseau horaire du serveur de la ressource.
func (s *Store) EnabledScheduledTasks(ctx context.Context) ([]*ScheduledTask, []string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+prefixed(schedTaskCols, "t.")+`, coalesce(sv.settings->>'timezone', 'UTC')
		FROM scheduled_tasks t JOIN resources r ON r.id = t.resource_id JOIN destinations d ON d.id = r.destination_id
		JOIN servers sv ON sv.id = d.server_id WHERE t.enabled AND sv.is_usable`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var list []*ScheduledTask
	var tzs []string
	for rows.Next() {
		t := &ScheduledTask{}
		var tz string
		if err := rows.Scan(&t.ID, &t.TeamID, &t.ResourceID, &t.Name, &t.Command, &t.Frequency, &t.Container, &t.Timeout, &t.Enabled,
			&t.LastRunAt, &t.CreatedAt, &tz); err != nil {
			return nil, nil, err
		}
		list, tzs = append(list, t), append(tzs, tz)
	}
	return list, tzs, rows.Err()
}

func (s *Store) TouchScheduledTask(ctx context.Context, id string, at time.Time) error {
	_, err := s.Pool.Exec(ctx, `UPDATE scheduled_tasks SET last_run_at = $2 WHERE id = $1`, id, at)
	return err
}

func (s *Store) DeleteScheduledTask(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM scheduled_tasks WHERE team_id = $1 AND id = $2`, teamID, id)
	return err
}
