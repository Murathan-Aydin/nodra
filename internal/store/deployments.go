package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Deployment struct {
	ID              string     `json:"id"`
	TeamID          string     `json:"team_id"`
	ApplicationID   string     `json:"application_id"`
	ServerID        string     `json:"server_id"`
	Status          string     `json:"status"`
	Trigger         string     `json:"trigger"`
	Branch          string     `json:"branch"`
	CommitSHA       string     `json:"commit_sha"`
	CommitMessage   string     `json:"commit_message"`
	PRNumber        int        `json:"pr_number"`
	ForceRebuild    bool       `json:"force_rebuild"`
	RollbackImage   string     `json:"rollback_image"`
	Image           string     `json:"image"`
	Error           string     `json:"error"`
	CancelRequested bool       `json:"cancel_requested"`
	CreatedBy       *string    `json:"created_by"`
	CreatedAt       time.Time  `json:"created_at"`
	StartedAt       *time.Time `json:"started_at"`
	FinishedAt      *time.Time `json:"finished_at"`
}

const depCols = `id::text, team_id::text, application_id::text, server_id::text, status, trigger, branch, commit_sha, commit_message,
	pr_number, force_rebuild, rollback_image, image, error, cancel_requested, created_by::text, created_at, started_at, finished_at`

func scanDeployment(row pgx.Row) (*Deployment, error) {
	d := &Deployment{}
	err := row.Scan(&d.ID, &d.TeamID, &d.ApplicationID, &d.ServerID, &d.Status, &d.Trigger, &d.Branch, &d.CommitSHA, &d.CommitMessage,
		&d.PRNumber, &d.ForceRebuild, &d.RollbackImage, &d.Image, &d.Error, &d.CancelRequested, &d.CreatedBy, &d.CreatedAt,
		&d.StartedAt, &d.FinishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// CreateDeployment insère un déploiement (dans tx pour l'enfiler avec son job).
func (s *Store) CreateDeployment(ctx context.Context, tx pgx.Tx, d *Deployment) (*Deployment, error) {
	return scanDeployment(tx.QueryRow(ctx, `INSERT INTO deployments (team_id, application_id, server_id, trigger, branch, commit_sha,
		pr_number, force_rebuild, rollback_image, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING `+depCols,
		d.TeamID, d.ApplicationID, d.ServerID, d.Trigger, d.Branch, d.CommitSHA, d.PRNumber, d.ForceRebuild, d.RollbackImage, d.CreatedBy))
}

func (s *Store) DeploymentByID(ctx context.Context, id string) (*Deployment, error) {
	return scanDeployment(s.Pool.QueryRow(ctx, `SELECT `+depCols+` FROM deployments WHERE id = $1`, id))
}

func (s *Store) ListDeployments(ctx context.Context, appID string, limit int) ([]*Deployment, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+depCols+` FROM deployments WHERE application_id = $1 ORDER BY created_at DESC LIMIT $2`, appID, limit)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanDeployment)
}

// ClaimDeployment démarre un déploiement en file si aucun autre n'est en cours pour la même application
// (et la même PR) et si le serveur n'a pas atteint sa limite de builds simultanés.
// Renvoie (démarré, toujours en file).
func (s *Store) ClaimDeployment(ctx context.Context, id string, maxConcurrent int) (bool, bool, error) {
	var claimed, queued bool
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		var server string
		if err := tx.QueryRow(ctx, `SELECT server_id::text FROM deployments WHERE id = $1`, id).Scan(&server); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, server); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `UPDATE deployments d SET status = 'in_progress', started_at = now()
			WHERE d.id = $1 AND d.status = 'queued'
			  AND NOT EXISTS (SELECT 1 FROM deployments o WHERE o.application_id = d.application_id AND o.pr_number = d.pr_number
			                  AND o.status = 'in_progress')
			  AND (SELECT count(*) FROM deployments o WHERE o.server_id = d.server_id AND o.status = 'in_progress') < $2`,
			id, maxConcurrent)
		if err != nil {
			return err
		}
		claimed = tag.RowsAffected() == 1
		if !claimed {
			return tx.QueryRow(ctx, `SELECT status = 'queued' AND NOT cancel_requested FROM deployments WHERE id = $1`, id).Scan(&queued)
		}
		return nil
	})
	return claimed, queued, err
}

func (s *Store) FinishDeployment(ctx context.Context, id, status, image, errMsg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE deployments SET status = $2, image = CASE WHEN $3 = '' THEN image ELSE $3 END, error = $4,
		finished_at = now() WHERE id = $1 AND status IN ('queued', 'in_progress')`, id, status, image, errMsg)
	return err
}

func (s *Store) SetDeploymentCommit(ctx context.Context, id, sha, message string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE deployments SET commit_sha = $2, commit_message = $3 WHERE id = $1`, id, sha, message)
	return err
}

// RequestCancel annule un déploiement en file, ou demande l'arrêt d'un déploiement en cours.
func (s *Store) RequestCancel(ctx context.Context, id string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE deployments SET cancel_requested = true,
		status = CASE WHEN status = 'queued' THEN 'cancelled' ELSE status END,
		finished_at = CASE WHEN status = 'queued' THEN now() ELSE finished_at END
		WHERE id = $1 AND status IN ('queued', 'in_progress')`, id)
	return tag.RowsAffected() == 1, err
}

// CancelPreviewDeployments annule les déploiements en attente ou en cours d'une preview (PR fermée).
func (s *Store) CancelPreviewDeployments(ctx context.Context, appID string, pr int) error {
	_, err := s.Pool.Exec(ctx, `UPDATE deployments SET cancel_requested = true,
		status = CASE WHEN status = 'queued' THEN 'cancelled' ELSE status END,
		finished_at = CASE WHEN status = 'queued' THEN now() ELSE finished_at END
		WHERE application_id = $1 AND pr_number = $2 AND status IN ('queued', 'in_progress')`, appID, pr)
	return err
}

func (s *Store) CancelRequested(ctx context.Context, id string) (bool, error) {
	var c bool
	err := s.Pool.QueryRow(ctx, `SELECT cancel_requested FROM deployments WHERE id = $1`, id).Scan(&c)
	return c, err
}

// LastDeployedImage : image du dernier déploiement réussi (hors PR).
func (s *Store) LastDeployedImage(ctx context.Context, appID string, pr int) (string, error) {
	var img string
	err := s.Pool.QueryRow(ctx, `SELECT image FROM deployments WHERE application_id = $1 AND pr_number = $2 AND status = 'finished'
		AND image <> '' ORDER BY finished_at DESC LIMIT 1`, appID, pr).Scan(&img)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return img, err
}

// FailStaleDeployments marque en échec les déploiements interrompus (processus arrêté).
func (s *Store) FailStaleDeployments(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `UPDATE deployments SET status = 'failed', error = 'déploiement interrompu', finished_at = now()
		WHERE (status = 'in_progress' AND started_at < now() - interval '1 day') OR (status = 'queued' AND created_at < now() - interval '1 day')`)
	return err
}

type DeploymentLog struct {
	ID        int64     `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Kind      string    `json:"kind"`
	Hidden    bool      `json:"hidden"`
	Message   string    `json:"message"`
}

// AppendDeploymentLogs insère un lot de lignes.
func (s *Store) AppendDeploymentLogs(ctx context.Context, deploymentID string, lines []DeploymentLog) error {
	if len(lines) == 0 {
		return nil
	}
	b := &pgx.Batch{}
	for _, l := range lines {
		b.Queue(`INSERT INTO deployment_logs (deployment_id, created_at, kind, hidden, message) VALUES ($1, $2, $3, $4, $5)`,
			deploymentID, l.CreatedAt, l.Kind, l.Hidden, l.Message)
	}
	return s.Pool.SendBatch(ctx, b).Close()
}

func (s *Store) DeploymentLogs(ctx context.Context, deploymentID string, after int64, includeHidden bool, limit int) ([]DeploymentLog, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, created_at, kind, hidden, message FROM deployment_logs
		WHERE deployment_id = $1 AND id > $2 AND (NOT hidden OR $3) ORDER BY id LIMIT $4`, deploymentID, after, includeHidden, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (DeploymentLog, error) {
		var l DeploymentLog
		err := row.Scan(&l.ID, &l.CreatedAt, &l.Kind, &l.Hidden, &l.Message)
		return l, err
	})
}

// --- Statut des conteneurs ---

// SetResourceStatuses met à jour le statut des applications et des bases d'un serveur (hors PR) ;
// celles absentes de statuses passent à « stopped ».
func (s *Store) SetResourceStatuses(ctx context.Context, serverID string, statuses map[string]string) error {
	rows, err := s.Pool.Query(ctx, `SELECT x.id::text, x.status, x.kind FROM (
			SELECT id, status, destination_id, 'applications' AS kind FROM applications
			UNION ALL SELECT id, status, destination_id, 'databases' FROM databases
			UNION ALL SELECT id, status, destination_id, 'services' FROM services
		) x JOIN destinations d ON d.id = x.destination_id WHERE d.server_id = $1`, serverID)
	if err != nil {
		return err
	}
	type row struct{ status, table string }
	current := map[string]row{}
	for rows.Next() {
		var id string
		var r row
		if err := rows.Scan(&id, &r.status, &r.table); err != nil {
			rows.Close()
			return err
		}
		current[id] = r
	}
	rows.Close()
	for id, old := range current {
		st, ok := statuses[id]
		if !ok {
			st = "stopped"
		}
		if st != old.status {
			// table : constante de la requête ci-dessus
			if _, err := s.Pool.Exec(ctx, `UPDATE `+old.table+` SET status = $2 WHERE id = $1`, id, st); err != nil {
				return err
			}
		}
	}
	return nil
}
