package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const TaskScheduled = "scheduled_task"

type ScheduledParams struct {
	ScheduledTaskID string `json:"scheduled_task_id"`
}

// ContainerFilter : filtre docker ps des conteneurs d'une ressource (service compose facultatif, hors PR).
func ContainerFilter(resourceID, service string) string {
	f := "--filter label=nodra.resource=" + resourceID + " --filter label=nodra.pr=0"
	if service != "" {
		f += " --filter label=nodra.service=" + ssh.Quote(service)
	}
	return f
}

// scheduledTask exécute la commande dans un conteneur en cours d'exécution de la ressource.
func (r *Runner) scheduledTask() servers.TaskFunc {
	return func(ctx context.Context, task *store.ServerTask, sv *store.Server, t ssh.Target, out io.Writer) error {
		var p ScheduledParams
		if err := json.Unmarshal(task.Params, &p); err != nil {
			return err
		}
		st, err := r.Store.ScheduledTaskByIDAny(ctx, p.ScheduledTaskID)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(ctx, time.Duration(st.Timeout)*time.Second)
		defer cancel()
		names, err := r.run(ctx, t, "docker ps "+ContainerFilter(st.ResourceID, st.Container)+" --filter status=running --format '{{.Names}}'\n")
		if err != nil {
			return err
		}
		list := strings.Fields(names)
		switch {
		case len(list) == 0:
			return errors.New("aucun conteneur en cours d'exécution pour cette ressource")
		case len(list) > 1 && st.Container == "":
			return fmt.Errorf("plusieurs conteneurs (%s) : précisez le conteneur ciblé", strings.Join(list, ", "))
		}
		container := list[0]
		fmt.Fprintf(out, "$ %s (dans %s)\n", st.Command, container)
		code, err := r.SSH.RunInput(ctx, t, "sh -s", strings.NewReader("docker exec "+ssh.Quote(container)+" sh -c "+ssh.Quote(st.Command)+"\n"), out, out)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("délai dépassé (%d s)", st.Timeout)
		}
		if err != nil {
			return err
		}
		if code != 0 {
			return fmt.Errorf("la commande a échoué (code %d)", code)
		}
		return nil
	}
}
