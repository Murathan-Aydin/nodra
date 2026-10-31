package store

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// --- Stockages S3 ---

type S3Storage struct {
	ID           string     `json:"id"`
	TeamID       string     `json:"team_id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Endpoint     string     `json:"endpoint"`
	Region       string     `json:"region"`
	Bucket       string     `json:"bucket"`
	PathPrefix   string     `json:"path_prefix"`
	AccessKeyEnc string     `json:"-"`
	SecretKeyEnc string     `json:"-"`
	IsUsable     bool       `json:"is_usable"`
	LastError    string     `json:"last_error"`
	CheckedAt    *time.Time `json:"checked_at"`
	CreatedAt    time.Time  `json:"created_at"`
}

const s3Cols = `id::text, team_id::text, name, description, endpoint, region, bucket, path_prefix, access_key_enc, secret_key_enc,
	is_usable, last_error, checked_at, created_at`

func scanS3(row pgx.Row) (*S3Storage, error) {
	s := &S3Storage{}
	err := row.Scan(&s.ID, &s.TeamID, &s.Name, &s.Description, &s.Endpoint, &s.Region, &s.Bucket, &s.PathPrefix, &s.AccessKeyEnc,
		&s.SecretKeyEnc, &s.IsUsable, &s.LastError, &s.CheckedAt, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return s, err
}

func (s *Store) CreateS3Storage(ctx context.Context, x *S3Storage) (*S3Storage, error) {
	r, err := scanS3(s.Pool.QueryRow(ctx, `INSERT INTO s3_storages (team_id, name, description, endpoint, region, bucket, path_prefix,
		access_key_enc, secret_key_enc) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING `+s3Cols,
		x.TeamID, x.Name, x.Description, x.Endpoint, x.Region, x.Bucket, x.PathPrefix, x.AccessKeyEnc, x.SecretKeyEnc))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return r, err
}

func (s *Store) UpdateS3Storage(ctx context.Context, x *S3Storage) error {
	_, err := s.Pool.Exec(ctx, `UPDATE s3_storages SET name = $2, description = $3, endpoint = $4, region = $5, bucket = $6,
		path_prefix = $7, access_key_enc = $8, secret_key_enc = $9, updated_at = now() WHERE id = $1`,
		x.ID, x.Name, x.Description, x.Endpoint, x.Region, x.Bucket, x.PathPrefix, x.AccessKeyEnc, x.SecretKeyEnc)
	if isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) SetS3Check(ctx context.Context, id string, usable bool, errMsg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE s3_storages SET is_usable = $2, last_error = $3, checked_at = now() WHERE id = $1`, id, usable, errMsg)
	return err
}

func (s *Store) ListS3Storages(ctx context.Context, teamID string) ([]*S3Storage, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+s3Cols+` FROM s3_storages WHERE team_id = $1 ORDER BY lower(name)`, teamID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanS3)
}

func (s *Store) S3StorageByID(ctx context.Context, teamID, id string) (*S3Storage, error) {
	return scanS3(s.Pool.QueryRow(ctx, `SELECT `+s3Cols+` FROM s3_storages WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) S3StorageByIDAny(ctx context.Context, id string) (*S3Storage, error) {
	return scanS3(s.Pool.QueryRow(ctx, `SELECT `+s3Cols+` FROM s3_storages WHERE id = $1`, id))
}

func (s *Store) DeleteS3Storage(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM s3_storages WHERE team_id = $1 AND id = $2`, teamID, id)
	return err
}

// --- Planifications ---

type BackupSchedule struct {
	ID            string     `json:"id"`
	TeamID        string     `json:"team_id"`
	Kind          string     `json:"kind"`
	ResourceID    string     `json:"resource_id"`
	VolumeName    string     `json:"volume_name"`
	Databases     string     `json:"databases"`
	DumpAll       bool       `json:"dump_all"`
	Frequency     string     `json:"frequency"`
	Enabled       bool       `json:"enabled"`
	KeepLocal     int        `json:"keep_local"`
	KeepLocalDays int        `json:"keep_local_days"`
	LocalBackup   bool       `json:"local_backup"`
	S3StorageID   *string    `json:"s3_storage_id"`
	KeepS3        int        `json:"keep_s3"`
	KeepS3Days    int        `json:"keep_s3_days"`
	LastRunAt     *time.Time `json:"last_run_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

const scheduleCols = `id::text, team_id::text, kind, resource_id::text, volume_name, databases, dump_all, frequency, enabled,
	keep_local, keep_local_days, local_backup, s3_storage_id::text, keep_s3, keep_s3_days, last_run_at, created_at`

func scanSchedule(row pgx.Row) (*BackupSchedule, error) {
	b := &BackupSchedule{}
	err := row.Scan(&b.ID, &b.TeamID, &b.Kind, &b.ResourceID, &b.VolumeName, &b.Databases, &b.DumpAll, &b.Frequency, &b.Enabled,
		&b.KeepLocal, &b.KeepLocalDays, &b.LocalBackup, &b.S3StorageID, &b.KeepS3, &b.KeepS3Days, &b.LastRunAt, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return b, err
}

func scheduleArgs(b *BackupSchedule) []any {
	return []any{b.TeamID, b.Kind, b.ResourceID, b.VolumeName, b.Databases, b.DumpAll, b.Frequency, b.Enabled, b.KeepLocal,
		b.KeepLocalDays, b.LocalBackup, b.S3StorageID, b.KeepS3, b.KeepS3Days}
}

func (s *Store) CreateBackupSchedule(ctx context.Context, b *BackupSchedule) (*BackupSchedule, error) {
	return scanSchedule(s.Pool.QueryRow(ctx, `INSERT INTO backup_schedules (team_id, kind, resource_id, volume_name, databases, dump_all,
		frequency, enabled, keep_local, keep_local_days, local_backup, s3_storage_id, keep_s3, keep_s3_days)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING `+scheduleCols, scheduleArgs(b)...))
}

func (s *Store) UpdateBackupSchedule(ctx context.Context, b *BackupSchedule) error {
	_, err := s.Pool.Exec(ctx, `UPDATE backup_schedules SET volume_name = $2, databases = $3, dump_all = $4, frequency = $5,
		enabled = $6, keep_local = $7, keep_local_days = $8, local_backup = $9, s3_storage_id = $10, keep_s3 = $11,
		keep_s3_days = $12, updated_at = now() WHERE id = $1`, b.ID, b.VolumeName, b.Databases, b.DumpAll, b.Frequency, b.Enabled,
		b.KeepLocal, b.KeepLocalDays, b.LocalBackup, b.S3StorageID, b.KeepS3, b.KeepS3Days)
	return err
}

func (s *Store) BackupScheduleByID(ctx context.Context, teamID, id string) (*BackupSchedule, error) {
	return scanSchedule(s.Pool.QueryRow(ctx, `SELECT `+scheduleCols+` FROM backup_schedules WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) BackupScheduleByIDAny(ctx context.Context, id string) (*BackupSchedule, error) {
	return scanSchedule(s.Pool.QueryRow(ctx, `SELECT `+scheduleCols+` FROM backup_schedules WHERE id = $1`, id))
}

func (s *Store) ListBackupSchedules(ctx context.Context, resourceID string) ([]*BackupSchedule, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+scheduleCols+` FROM backup_schedules WHERE resource_id = $1 ORDER BY created_at`, resourceID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanSchedule)
}

// EnabledBackupSchedules : planifications actives avec le fuseau horaire du serveur de la ressource.
func (s *Store) EnabledBackupSchedules(ctx context.Context) ([]*BackupSchedule, []string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+prefixed(scheduleCols, "b.")+`, coalesce(sv.settings->>'timezone', 'UTC')
		FROM backup_schedules b JOIN resources r ON r.id = b.resource_id JOIN destinations d ON d.id = r.destination_id
		JOIN servers sv ON sv.id = d.server_id WHERE b.enabled AND sv.is_usable`)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var list []*BackupSchedule
	var tzs []string
	for rows.Next() {
		b := &BackupSchedule{}
		var tz string
		if err := rows.Scan(&b.ID, &b.TeamID, &b.Kind, &b.ResourceID, &b.VolumeName, &b.Databases, &b.DumpAll, &b.Frequency, &b.Enabled,
			&b.KeepLocal, &b.KeepLocalDays, &b.LocalBackup, &b.S3StorageID, &b.KeepS3, &b.KeepS3Days, &b.LastRunAt, &b.CreatedAt, &tz); err != nil {
			return nil, nil, err
		}
		list, tzs = append(list, b), append(tzs, tz)
	}
	return list, tzs, rows.Err()
}

func (s *Store) TouchBackupSchedule(ctx context.Context, id string, at time.Time) error {
	_, err := s.Pool.Exec(ctx, `UPDATE backup_schedules SET last_run_at = $2 WHERE id = $1`, id, at)
	return err
}

func (s *Store) DeleteBackupSchedule(ctx context.Context, teamID, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM backup_schedules WHERE team_id = $1 AND id = $2`, teamID, id)
	return err
}

// --- Exécutions ---

type BackupExecution struct {
	ID           string     `json:"id"`
	ScheduleID   *string    `json:"schedule_id"`
	TeamID       string     `json:"team_id"`
	ResourceID   string     `json:"resource_id"`
	ServerID     string     `json:"server_id"`
	DatabaseName string     `json:"database_name"`
	Status       string     `json:"status"`
	Filename     string     `json:"filename"`
	Size         int64      `json:"size"`
	S3StorageID  *string    `json:"s3_storage_id"`
	S3Key        string     `json:"s3_key"`
	LocalDeleted bool       `json:"local_deleted"`
	S3Deleted    bool       `json:"s3_deleted"`
	Message      string     `json:"message"`
	CreatedAt    time.Time  `json:"created_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

const execCols = `id::text, schedule_id::text, team_id::text, resource_id::text, server_id::text, database_name, status, filename, size,
	s3_storage_id::text, s3_key, local_deleted, s3_deleted, message, created_at, finished_at`

func scanExecution(row pgx.Row) (*BackupExecution, error) {
	e := &BackupExecution{}
	err := row.Scan(&e.ID, &e.ScheduleID, &e.TeamID, &e.ResourceID, &e.ServerID, &e.DatabaseName, &e.Status, &e.Filename, &e.Size,
		&e.S3StorageID, &e.S3Key, &e.LocalDeleted, &e.S3Deleted, &e.Message, &e.CreatedAt, &e.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return e, err
}

func (s *Store) CreateBackupExecution(ctx context.Context, e *BackupExecution) (*BackupExecution, error) {
	return scanExecution(s.Pool.QueryRow(ctx, `INSERT INTO backup_executions (schedule_id, team_id, resource_id, server_id, database_name,
		filename, s3_storage_id) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+execCols,
		e.ScheduleID, e.TeamID, e.ResourceID, e.ServerID, e.DatabaseName, e.Filename, e.S3StorageID))
}

func (s *Store) FinishBackupExecution(ctx context.Context, e *BackupExecution) error {
	_, err := s.Pool.Exec(ctx, `UPDATE backup_executions SET status = $2, size = $3, s3_key = $4, local_deleted = $5, message = $6,
		finished_at = now() WHERE id = $1`, e.ID, e.Status, e.Size, e.S3Key, e.LocalDeleted, e.Message)
	return err
}

func (s *Store) MarkBackupDeleted(ctx context.Context, id string, local, s3 bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE backup_executions SET local_deleted = local_deleted OR $2, s3_deleted = s3_deleted OR $3
		WHERE id = $1`, id, local, s3)
	return err
}

func (s *Store) BackupExecutionByID(ctx context.Context, teamID, id string) (*BackupExecution, error) {
	return scanExecution(s.Pool.QueryRow(ctx, `SELECT `+execCols+` FROM backup_executions WHERE team_id = $1 AND id = $2`, teamID, id))
}

func (s *Store) BackupExecutionByIDAny(ctx context.Context, id string) (*BackupExecution, error) {
	return scanExecution(s.Pool.QueryRow(ctx, `SELECT `+execCols+` FROM backup_executions WHERE id = $1`, id))
}

func (s *Store) ListBackupExecutions(ctx context.Context, resourceID string, limit int) ([]*BackupExecution, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+execCols+` FROM backup_executions WHERE resource_id = $1 ORDER BY created_at DESC LIMIT $2`,
		resourceID, limit)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanExecution)
}

// ScheduleExecutions : exécutions réussies d'une planification (plus récentes d'abord), pour la rétention.
func (s *Store) ScheduleExecutions(ctx context.Context, scheduleID, databaseName string) ([]*BackupExecution, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+execCols+` FROM backup_executions WHERE schedule_id = $1 AND database_name = $2
		AND status = 'success' ORDER BY created_at DESC`, scheduleID, databaseName)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanExecution)
}

// FailStaleBackups marque en échec les sauvegardes restées en cours (processus arrêté).
func (s *Store) FailStaleBackups(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE backup_executions SET status = 'failed', message = 'sauvegarde interrompue', finished_at = now()
		WHERE status = 'running' AND created_at < now() - interval '1 day'`)
	return err
}

// prefixed préfixe chaque colonne d'une liste (ex. "b.") pour une requête avec jointure.
func prefixed(cols, p string) string {
	parts := strings.Split(cols, ",")
	for i, c := range parts {
		parts[i] = p + strings.TrimSpace(c)
	}
	return strings.Join(parts, ", ")
}
