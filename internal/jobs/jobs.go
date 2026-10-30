// Package jobs exécute les tâches de fond avec River (file de jobs dans PostgreSQL) :
// tâches serveur, collecte des métriques, nettoyage Docker planifié, maintenance.
package jobs

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"

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
}

func (w *maintenanceWorker) Work(ctx context.Context, _ *river.Job[MaintenanceArgs]) error {
	for _, f := range []func(context.Context) error{
		w.st.DeleteExpiredSessions, w.st.PurgeServerMetrics, w.st.ExpireApprovalRequests, w.st.FailStaleServerTasks,
	} {
		if err := f(ctx); err != nil {
			return err
		}
	}
	return nil
}

// --- Client ---

type Options struct {
	Periodic bool // planifier les jobs périodiques (désactivé dans les tests)
}

type Jobs struct {
	Client *river.Client[pgx.Tx]
	st     *store.Store
	log    *slog.Logger
}

func New(st *store.Store, svc *servers.Service, log *slog.Logger, opts Options) (*Jobs, error) {
	j := &Jobs{st: st, log: log}
	workers := river.NewWorkers()
	river.AddWorker(workers, &serverTaskWorker{svc: svc})
	river.AddWorker(workers, &metricsWorker{svc: svc})
	river.AddWorker(workers, &cleanupSchedulerWorker{j: j})
	river.AddWorker(workers, &maintenanceWorker{st: st})

	cfg := &river.Config{
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
			"servers":          {MaxWorkers: 20},
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
			river.NewPeriodicJob(river.PeriodicInterval(time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return MaintenanceArgs{}, nil }, &river.PeriodicJobOpts{ID: "maintenance", RunOnStart: true}),
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
