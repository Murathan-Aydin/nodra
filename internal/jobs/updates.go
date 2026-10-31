package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/riverqueue/river"
	"github.com/robfig/cron/v3"

	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/store"
	"github.com/Murathan-Aydin/nodra/internal/version"
)

// UpdateCheckArgs : vérification périodique des nouvelles versions (selon update_check_frequency).
type UpdateCheckArgs struct{}

func (UpdateCheckArgs) Kind() string                 { return "update_check" }
func (UpdateCheckArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 1} }

type updateCheckWorker struct {
	river.WorkerDefaults[UpdateCheckArgs]
	st     *store.Store
	j      *Jobs
	api    string
	srcDir string // dossier de l'installation (update.sh) sur l'hôte
}

func (w *updateCheckWorker) Work(ctx context.Context, _ *river.Job[UpdateCheckArgs]) error {
	now := time.Now()
	if _, err := CheckUpdates(ctx, w.st, w.api, now); err != nil {
		return err
	}
	return w.autoUpdate(ctx, now)
}

var updateCron = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// AutoUpdateDue indique si la mise à jour automatique doit partir maintenant : activée, version plus
// récente connue, échéance cron (auto_update_frequency, fuseau de l'instance) dans la dernière fenêtre.
func AutoUpdateDue(i *store.InstanceSettings, now time.Time) bool {
	if !i.AutoUpdateEnabled || !version.Newer(i.LatestVersion, version.Version) {
		return false
	}
	sched, err := updateCron.Parse(i.AutoUpdateFrequency)
	if err != nil {
		return false
	}
	loc, err := time.LoadLocation(i.Timezone)
	if err != nil {
		loc = time.UTC
	}
	return !sched.Next(now.Add(-updateWindow).In(loc)).After(now)
}

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

// StartInstanceUpdate lance la mise à jour de l'instance (update.sh) sur le serveur localhost.
func StartInstanceUpdate(ctx context.Context, st *store.Store, enqueue func(context.Context, *store.ServerTask) (*store.ServerTask, error),
	srcDir, tag string, by *string) (*store.ServerTask, error) {
	id, err := st.LocalhostServerID(ctx)
	if err != nil {
		return nil, err
	}
	if id == "" {
		return nil, ErrNoLocalhost
	}
	sv, err := st.ServerByIDAny(ctx, id)
	if err != nil {
		return nil, err
	}
	params, _ := json.Marshal(servers.CommandParams{Command: servers.InstanceUpdateCommand(srcDir, tag)})
	return enqueue(ctx, &store.ServerTask{TeamID: sv.TeamID, ServerID: sv.ID, Kind: servers.TaskCommand, Params: params, CreatedBy: by})
}

// ErrNoLocalhost : pas de serveur localhost (installation hors install.sh) : mise à jour à faire à la main.
var ErrNoLocalhost = errors.New("serveur localhost absent : mettez à jour l'instance à la main (update.sh)")

// autoUpdate lance la mise à jour si elle est activée, qu'une version plus récente existe et que la
// fenêtre cron (auto_update_frequency) vient de s'ouvrir ; une seule tentative par version et par jour.
func (w *updateCheckWorker) autoUpdate(ctx context.Context, now time.Time) error {
	i, err := w.st.InstanceSettings(ctx)
	if err != nil || w.srcDir == "" || !AutoUpdateDue(i, now) {
		return err
	}
	if ok, err := w.st.Throttle(ctx, "auto_update:"+i.LatestVersion, 24*time.Hour); err != nil || !ok {
		return err
	}
	_, err = StartInstanceUpdate(ctx, w.st, w.j.EnqueueServerTask, w.srcDir, i.LatestVersion, nil)
	if errors.Is(err, ErrNoLocalhost) {
		return nil
	}
	return err
}

// updateWindow : intervalle du job périodique ; une échéance cron tombée dans cette fenêtre déclenche la mise à jour.
const updateWindow = 10 * time.Minute
