// Package jobs exécute les tâches de fond avec River (file de jobs dans PostgreSQL) :
// tâches serveur, collecte des métriques, nettoyage Docker planifié, maintenance.
package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

	"github.com/Murathan-Aydin/nodra/internal/deploy"
	"github.com/Murathan-Aydin/nodra/internal/notify"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// Migrate crée ou met à jour les tables de River.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	_, err = m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	return err
}

// --- Arguments des jobs ---

type ServerTaskArgs struct {
	TaskID string `json:"task_id"`
}

func (ServerTaskArgs) Kind() string { return "server_task" }
func (ServerTaskArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "servers", MaxAttempts: 1}
}

// DeployArgs : déploiement d'une application ; Timeout en secondes (réglage du serveur).
type DeployArgs struct {
	DeploymentID string `json:"deployment_id"`
	Timeout      int    `json:"timeout"`
}

func (DeployArgs) Kind() string { return "deploy" }
func (DeployArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "deployments", MaxAttempts: 1}
}

type deployWorker struct {
	river.WorkerDefaults[DeployArgs]
	st     *store.Store
	runner *deploy.Runner
}

// Work réserve le déploiement (un seul à la fois par application, limite de builds par serveur) ;
// sinon le job est reporté sans consommer de tentative.
func (w *deployWorker) Work(ctx context.Context, job *river.Job[DeployArgs]) error {
	d, err := w.st.DeploymentByID(ctx, job.Args.DeploymentID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	limit := 2
	if sv, err := w.st.ServerByIDAny(ctx, d.ServerID); err == nil {
		limit = sv.Settings.ConcurrentBuilds
	}
	claimed, queued, err := w.st.ClaimDeployment(ctx, d.ID, limit)
	if err != nil {
		return err
	}
	if !claimed {
		if queued {
			return river.JobSnooze(3 * time.Second)
		}
		return nil // annulé
	}
	return w.runner.Run(ctx, d.ID)
}

func (w *deployWorker) Timeout(job *river.Job[DeployArgs]) time.Duration {
	if job.Args.Timeout < 60 {
		return time.Hour
	}
	return time.Duration(job.Args.Timeout) * time.Second
}

type BackupSchedulerArgs struct{}

func (BackupSchedulerArgs) Kind() string                 { return "scheduler" }
func (BackupSchedulerArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 1} }

type backupSchedulerWorker struct {
	river.WorkerDefaults[BackupSchedulerArgs]
	j *Jobs
}

func (w *backupSchedulerWorker) Work(ctx context.Context, _ *river.Job[BackupSchedulerArgs]) error {
	return w.j.RunSchedules(ctx, time.Now())
}

// NotifyArgs : diffusion d'une notification aux canaux de l'équipe.
type NotifyArgs struct {
	Event notify.Event `json:"event"`
}

func (NotifyArgs) Kind() string { return "notify" }
func (NotifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: "notifications", MaxAttempts: 3}
}

type notifyWorker struct {
	river.WorkerDefaults[NotifyArgs]
	d *notify.Dispatcher
}

func (w *notifyWorker) Work(ctx context.Context, job *river.Job[NotifyArgs]) error {
	if w.d == nil {
		return nil
	}
	return w.d.Dispatch(ctx, job.Args.Event)
}

type MetricsArgs struct{}

func (MetricsArgs) Kind() string { return "collect_metrics" }
func (MetricsArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByPeriod: time.Minute}}
}

type CleanupSchedulerArgs struct{}

func (CleanupSchedulerArgs) Kind() string                 { return "docker_cleanup_scheduler" }
func (CleanupSchedulerArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 1} }

type MaintenanceArgs struct{}

func (MaintenanceArgs) Kind() string                 { return "maintenance" }
func (MaintenanceArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 3} }

// --- Workers ---

type serverTaskWorker struct {
	river.WorkerDefaults[ServerTaskArgs]
	svc *servers.Service
}

func (w *serverTaskWorker) Work(ctx context.Context, job *river.Job[ServerTaskArgs]) error {
	return w.svc.RunTask(ctx, job.Args.TaskID)
}

// Timeout : l'installation de Docker ou une mise à jour système peut être longue.
func (w *serverTaskWorker) Timeout(*river.Job[ServerTaskArgs]) time.Duration { return 2 * time.Hour }

type metricsWorker struct {
	river.WorkerDefaults[MetricsArgs]
	svc *servers.Service
}

func (w *metricsWorker) Work(ctx context.Context, _ *river.Job[MetricsArgs]) error {
	return w.svc.CollectMetrics(ctx)
}

type cleanupSchedulerWorker struct {
	river.WorkerDefaults[CleanupSchedulerArgs]
	j *Jobs
}

func (w *cleanupSchedulerWorker) Work(ctx context.Context, _ *river.Job[CleanupSchedulerArgs]) error {
	return w.j.ScheduleCleanups(ctx, time.Now())
}

type maintenanceWorker struct {
	river.WorkerDefaults[MaintenanceArgs]
	st *store.Store
	j  *Jobs
}

func (w *maintenanceWorker) Work(ctx context.Context, _ *river.Job[MaintenanceArgs]) error {
	for _, f := range []func(context.Context) error{
		w.st.DeleteExpiredSessions, w.st.PurgeServerMetrics, w.st.ExpireApprovalRequests, w.st.FailStaleServerTasks,
		w.st.FailStaleDeployments, w.st.FailStaleBackups, w.st.PurgeDeliveries, w.st.PurgeOAuthStates, w.j.notifyExpirations,
	} {
		if err := f(ctx); err != nil {
			return err
		}
	}
	return nil
}

// --- Client ---

type Options struct {
	Periodic bool               // planifier les jobs périodiques (désactivé dans les tests)
	Notifier *notify.Dispatcher // diffusion des notifications (nil : ignorées)
	// ReleasesAPI : API des releases interrogée pour les mises à jour (GitHub par défaut).
	ReleasesAPI string
	// InstallDir : dossier de l'installation sur l'hôte (update.sh) pour la mise à jour automatique.
	InstallDir string
}

type Jobs struct {
	Client *river.Client[pgx.Tx]
	st     *store.Store
	log    *slog.Logger
}

func New(st *store.Store, svc *servers.Service, runner *deploy.Runner, log *slog.Logger, opts Options) (*Jobs, error) {
	j := &Jobs{st: st, log: log}
	workers := river.NewWorkers()
	river.AddWorker(workers, &serverTaskWorker{svc: svc})
	river.AddWorker(workers, &deployWorker{st: st, runner: runner})
	river.AddWorker(workers, &metricsWorker{svc: svc})
	river.AddWorker(workers, &cleanupSchedulerWorker{j: j})
	river.AddWorker(workers, &backupSchedulerWorker{j: j})
	river.AddWorker(workers, &notifyWorker{d: opts.Notifier})
	river.AddWorker(workers, &maintenanceWorker{st: st, j: j})
	river.AddWorker(workers, &updateCheckWorker{st: st, j: j, api: opts.ReleasesAPI, srcDir: opts.InstallDir})

	cfg := &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
			"servers":          {MaxWorkers: 20},
			"deployments":      {MaxWorkers: 50},
			"notifications":    {MaxWorkers: 10},
		},
		Workers: workers,
		Logger:  log,
	}
	if opts.Periodic {
		cfg.PeriodicJobs = []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(time.Minute),
				func() (river.JobArgs, *river.InsertOpts) { return MetricsArgs{}, nil }, &river.PeriodicJobOpts{ID: "metrics"}),
			river.NewPeriodicJob(river.PeriodicInterval(5*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) { return CleanupSchedulerArgs{}, nil }, &river.PeriodicJobOpts{ID: "cleanup"}),
			river.NewPeriodicJob(river.PeriodicInterval(time.Minute),
				func() (river.JobArgs, *river.InsertOpts) { return BackupSchedulerArgs{}, nil }, &river.PeriodicJobOpts{ID: "scheduler"}),
			river.NewPeriodicJob(river.PeriodicInterval(time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return MaintenanceArgs{}, nil }, &river.PeriodicJobOpts{ID: "maintenance", RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(10*time.Minute),
				func() (river.JobArgs, *river.InsertOpts) { return UpdateCheckArgs{}, nil }, &river.PeriodicJobOpts{ID: "update_check", RunOnStart: true}),
		}
	}
	client, err := river.NewClient(riverpgxv5.New(st.Pool), cfg)
	if err != nil {
		return nil, err
	}
	j.Client = client
	return j, nil
}

func (j *Jobs) Start(ctx context.Context) error { return j.Client.Start(ctx) }

func (j *Jobs) Stop(ctx context.Context) error { return j.Client.Stop(ctx) }

// Publish enfile une notification (envoi asynchrone par le worker notify).
func (j *Jobs) Publish(ctx context.Context, e notify.Event) {
	if e.At.IsZero() {
		e.At = time.Now()
	}
	if _, err := j.Client.Insert(ctx, NotifyArgs{Event: e}, nil); err != nil {
		j.log.Error("notification non enfilée", "event", e.Type, "err", err)
	}
}

// notifyExpirations prévient (une fois par jour) des certificats et jetons API qui expirent bientôt.
func (j *Jobs) notifyExpirations(ctx context.Context) error {
	certs, teams, err := j.st.ExpiringCertificates(ctx, 14*24*time.Hour)
	if err != nil {
		return err
	}
	for i, c := range certs {
		if send, err := j.st.Throttle(ctx, "cert:"+c.ID, 24*time.Hour); err == nil && send {
			j.Publish(ctx, notify.Event{Type: notify.CertificateExpiring, TeamID: teams[i], Title: "Certificat bientôt expiré",
				Message: fmt.Sprintf("Le certificat %s (%s) expire le %s", c.Name, strings.Join(c.Domains, ", "), c.ExpiresAt.Format("02/01/2006")),
				Level:   "warning", URL: "/servers/" + c.ServerID})
		}
	}
	tokens, emails, err := j.st.ExpiringAPITokens(ctx, 7*24*time.Hour)
	if err != nil {
		return err
	}
	for i, t := range tokens {
		if send, err := j.st.Throttle(ctx, "token:"+t.ID, 24*time.Hour); err == nil && send {
			j.Publish(ctx, notify.Event{Type: notify.APITokenExpiring, TeamID: *t.TeamID, Title: "Jeton API bientôt expiré",
				Message: fmt.Sprintf("Le jeton « %s » de %s expire le %s", t.Name, emails[i], t.ExpiresAt.Format("02/01/2006")), Level: "warning"})
		}
	}
	return nil
}

// EnqueueDeployment crée le déploiement et son job dans une même transaction.
func (j *Jobs) EnqueueDeployment(ctx context.Context, d *store.Deployment, timeout int) (*store.Deployment, error) {
	var created *store.Deployment
	err := pgx.BeginFunc(ctx, j.st.Pool, func(tx pgx.Tx) error {
		var err error
		if created, err = j.st.CreateDeployment(ctx, tx, d); err != nil {
			return err
		}
		_, err = j.Client.InsertTx(ctx, tx, DeployArgs{DeploymentID: created.ID, Timeout: timeout}, nil)
		return err
	})
	return created, err
}

// EnqueueServerTask crée la tâche et son job dans une même transaction.
func (j *Jobs) EnqueueServerTask(ctx context.Context, t *store.ServerTask) (*store.ServerTask, error) {
	var created *store.ServerTask
	err := pgx.BeginFunc(ctx, j.st.Pool, func(tx pgx.Tx) error {
		var err error
		if created, err = j.st.CreateServerTask(ctx, tx, t); err != nil {
			return err
		}
		_, err = j.Client.InsertTx(ctx, tx, ServerTaskArgs{TaskID: created.ID}, nil)
		return err
	})
	return created, err
}
