package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/backup"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// ScheduleBackups enfile une sauvegarde pour chaque planification arrivée à échéance.
func (j *Jobs) ScheduleBackups(ctx context.Context, now time.Time) error {
	list, tzs, err := j.st.EnabledBackupSchedules(ctx)
	if err != nil {
		return err
	}
	for i, b := range list {
		if !backup.Due(b, tzs[i], now) {
			continue
		}
		if busy, err := j.st.HasActiveTaskWithParam(ctx, "schedule_id", b.ID); err != nil || busy {
			continue
		}
		if err := j.st.TouchBackupSchedule(ctx, b.ID, now); err != nil {
			return err
		}
		if _, err := j.EnqueueBackup(ctx, b, nil); err != nil {
			j.log.Error("planification de sauvegarde", "schedule", b.ID, "err", err)
		}
	}
	return nil
}

// EnqueueBackup lance une sauvegarde (tâche serveur sur le serveur de la ressource).
func (j *Jobs) EnqueueBackup(ctx context.Context, b *store.BackupSchedule, by *string) (*store.ServerTask, error) {
	loc, err := j.st.ResourceLocation(ctx, b.ResourceID)
	if err != nil {
		return nil, err
	}
	params, _ := json.Marshal(backup.RunParams{ScheduleID: b.ID})
	return j.EnqueueServerTask(ctx, &store.ServerTask{TeamID: b.TeamID, ServerID: loc.ServerID, Kind: backup.TaskBackupRun, Params: params, CreatedBy: by})
}
