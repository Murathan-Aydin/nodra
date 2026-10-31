package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"
	"github.com/robfig/cron/v3"

	"github.com/Murathan-Aydin/nodra/internal/store"
	"github.com/Murathan-Aydin/nodra/internal/version"
)

// UpdateCheckArgs : vérification périodique des nouvelles versions (selon update_check_frequency).
type UpdateCheckArgs struct{}

func (UpdateCheckArgs) Kind() string                 { return "update_check" }
func (UpdateCheckArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 1} }

type updateCheckWorker struct {
	river.WorkerDefaults[UpdateCheckArgs]
	st  *store.Store
	api string
}

func (w *updateCheckWorker) Work(ctx context.Context, _ *river.Job[UpdateCheckArgs]) error {
	_, err := CheckUpdates(ctx, w.st, w.api, time.Now())
	return err
}

var updateCron = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// CheckUpdates interroge les releases si l'échéance est passée ; renvoie true si la vérification a eu lieu.
func CheckUpdates(ctx context.Context, st *store.Store, api string, now time.Time) (bool, error) {
	i, err := st.InstanceSettings(ctx)
	if err != nil {
		return false, err
	}
	if i.UpdateCheckedAt != nil {
		sched, err := updateCron.Parse(i.UpdateCheckFrequency)
		if err != nil {
			return false, nil
		}
		loc, err := time.LoadLocation(i.Timezone)
		if err != nil {
			loc = time.UTC
		}
		if sched.Next(i.UpdateCheckedAt.In(loc)).After(now) {
			return false, nil
		}
	}
	latest, err := version.Latest(ctx, api)
	if err != nil {
		return false, err
	}
	return true, st.SetLatestVersion(ctx, latest)
}
