package jobs

import (
	"context"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

var cronParser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ParseCron valide une expression cron standard (5 champs) ou un descripteur (@daily…).
func ParseCron(expr string) (cron.Schedule, error) { return cronParser.Parse(expr) }

// CleanupDue indique si un nettoyage est dû : échéance cron dépassée depuis le dernier nettoyage,
// ou disque au-dessus du seuil (au plus un nettoyage par heure dans ce cas).
func CleanupDue(sv *store.Server, lastDiskPercent float64, now time.Time) bool {
	last := sv.CreatedAt
	if sv.LastCleanupAt != nil {
		last = *sv.LastCleanupAt
	}
	if sched, err := ParseCron(sv.Settings.DockerCleanupFrequency); err == nil {
		loc, err := time.LoadLocation(sv.Settings.Timezone)
		if err != nil {
			loc = time.UTC
		}
		if !sched.Next(last.In(loc)).After(now) {
			return true
		}
	}
	return sv.Settings.DockerCleanupThreshold > 0 && lastDiskPercent >= float64(sv.Settings.DockerCleanupThreshold) &&
		now.Sub(last) > time.Hour
}

// ScheduleCleanups enfile un nettoyage Docker pour chaque serveur dont le nettoyage est dû.
func (j *Jobs) ScheduleCleanups(ctx context.Context, now time.Time) error {
	list, err := j.st.UsableServers(ctx)
	if err != nil {
		return err
	}
	for _, sv := range list {
		disk := 0.0
		if ms, err := j.st.ServerMetrics(ctx, sv.ID, now.Add(-10*time.Minute)); err == nil && len(ms) > 0 {
			if last := ms[len(ms)-1]; last.DiskTotal > 0 {
				disk = 100 * float64(last.DiskUsed) / float64(last.DiskTotal)
			}
		}
		if !CleanupDue(sv, disk, now) {
			continue
		}
		busy, err := j.st.HasActiveServerTask(ctx, sv.ID, servers.TaskDockerCleanup)
		if err != nil || busy {
			continue
		}
		if _, err := j.EnqueueServerTask(ctx, &store.ServerTask{TeamID: sv.TeamID, ServerID: sv.ID, Kind: servers.TaskDockerCleanup}); err != nil {
			j.log.Error("planification du nettoyage", "server", sv.ID, "err", err)
		}
	}
	return nil
}
