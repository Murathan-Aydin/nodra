package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/deploy"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// ScheduleTasks enfile les tâches planifiées arrivées à échéance (une seule exécution à la fois par tâche).
func (j *Jobs) ScheduleTasks(ctx context.Context, now time.Time) error {
	list, tzs, err := j.st.EnabledScheduledTasks(ctx)
	if err != nil {
		return err
	}
	for i, t := range list {
		sched, err := cronParser.Parse(t.Frequency)
		if err != nil {
			continue
		}
		loc, err := time.LoadLocation(tzs[i])
		if err != nil {
			loc = time.UTC
		}
		last := t.CreatedAt
		if t.LastRunAt != nil {
			last = *t.LastRunAt
		}
		if sched.Next(last.In(loc)).After(now) {
			continue
		}
		if busy, err := j.st.HasActiveTaskWithParam(ctx, "scheduled_task_id", t.ID); err != nil || busy {
			continue
		}
		if err := j.st.TouchScheduledTask(ctx, t.ID, now); err != nil {
			return err
		}
		if _, err := j.EnqueueScheduledTask(ctx, t, nil); err != nil {
			j.log.Error("tâche planifiée", "task", t.ID, "err", err)
		}
	}
	return nil
}

// EnqueueScheduledTask lance une tâche planifiée sur le serveur de sa ressource.
func (j *Jobs) EnqueueScheduledTask(ctx context.Context, t *store.ScheduledTask, by *string) (*store.ServerTask, error) {
	loc, err := j.st.ResourceLocation(ctx, t.ResourceID)
	if err != nil {
		return nil, err
	}
	params, _ := json.Marshal(deploy.ScheduledParams{ScheduledTaskID: t.ID})
	return j.EnqueueServerTask(ctx, &store.ServerTask{TeamID: t.TeamID, ServerID: loc.ServerID, Kind: deploy.TaskScheduled, Params: params, CreatedBy: by})
}
