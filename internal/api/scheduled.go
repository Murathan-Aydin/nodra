package api

import (
	"net/http"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/backup"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func (s *Server) listScheduledTasks(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("resource_id")
	if !isUUID(id) {
		writeError(w, http.StatusBadRequest, "resource_id requis")
		return
	}
	if _, err := s.resourceType(r.Context(), id); err != nil {
		s.fail(w, err)
		return
	}
	if !s.authorize(w, r, "resource.read", resourceScope(id)) {
		return
	}
	list, err := s.Store.ListScheduledTasks(r.Context(), id)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type schedTaskInput struct {
	ResourceID string  `json:"resource_id"`
	Name       *string `json:"name"`
	Command    *string `json:"command"`
	Frequency  *string `json:"frequency"`
	Container  *string `json:"container"`
	Timeout    *int    `json:"timeout"`
	Enabled    *bool   `json:"enabled"`
}

func (in *schedTaskInput) apply(t *store.ScheduledTask) string {
	for _, f := range []struct {
		dst *string
		v   *string
	}{{&t.Name, in.Name}, {&t.Frequency, in.Frequency}, {&t.Container, in.Container}} {
		if f.v != nil {
			*f.dst = strings.TrimSpace(*f.v)
		}
	}
	if in.Command != nil {
		t.Command = strings.TrimSpace(*in.Command)
	}
	if in.Timeout != nil {
		t.Timeout = *in.Timeout
	}
	if in.Enabled != nil {
		t.Enabled = *in.Enabled
	}
	switch {
	case t.Name == "" || len(t.Name) > 100:
		return "nom requis (100 caractères max.)"
	case t.Command == "" || len(t.Command) > 10000:
		return "commande requise (10 000 caractères max.)"
	case backup.ParseCron(t.Frequency) != nil:
		return "frequency : expression cron invalide (ex. */5 * * * * ou @hourly)"
	case t.Container != "" && !serviceRe.MatchString(t.Container):
		return "nom de conteneur (service compose) invalide"
	case t.Timeout < 10 || t.Timeout > 86400:
		return "timeout entre 10 et 86400 secondes"
	}
	return ""
}

func (s *Server) createScheduledTask(w http.ResponseWriter, r *http.Request) {
	var in schedTaskInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	if !isUUID(in.ResourceID) {
		writeError(w, http.StatusUnprocessableEntity, "resource_id requis")
		return
	}
	if _, err := s.resourceType(ctx, in.ResourceID); err != nil {
		s.fail(w, err)
		return
	}
	// Exécuter une commande dans un conteneur équivaut à un accès terminal.
	if !s.authorize(w, r, "resource.terminal", resourceScope(in.ResourceID)) {
		return
	}
	t := &store.ScheduledTask{TeamID: teamFrom(ctx).Team.ID, ResourceID: in.ResourceID, Timeout: 3600, Enabled: true}
	if msg := in.apply(t); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	created, err := s.Store.CreateScheduledTask(ctx, t)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "scheduled_task.created", "resource", t.ResourceID, created)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) teamScheduledTask(w http.ResponseWriter, r *http.Request, perm string) (*store.ScheduledTask, bool) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	t, err := s.Store.ScheduledTaskByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return t, s.authorize(w, r, perm, resourceScope(t.ResourceID))
}

func (s *Server) updateScheduledTask(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamScheduledTask(w, r, "resource.terminal")
	if !ok {
		return
	}
	var in schedTaskInput
	if !decode(w, r, &in) {
		return
	}
	before := *t
	if msg := in.apply(t); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if err := s.Store.UpdateScheduledTask(r.Context(), t); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "scheduled_task.updated", "resource", t.ResourceID, map[string]any{"before": before, "after": t})
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteScheduledTask(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamScheduledTask(w, r, "resource.terminal")
	if !ok {
		return
	}
	if err := s.Store.DeleteScheduledTask(r.Context(), t.TeamID, t.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "scheduled_task.deleted", "resource", t.ResourceID, map[string]string{"name": t.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) runScheduledTask(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamScheduledTask(w, r, "resource.terminal")
	if !ok {
		return
	}
	ctx := r.Context()
	if busy, err := s.Store.HasActiveTaskWithParam(ctx, "scheduled_task_id", t.ID); err != nil {
		s.internal(w, err)
		return
	} else if busy {
		writeError(w, http.StatusConflict, "une exécution est déjà en cours")
		return
	}
	uid := userFrom(ctx).ID
	task, err := s.Jobs.EnqueueScheduledTask(ctx, t, &uid)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "scheduled_task.run", "resource", t.ResourceID, map[string]string{"scheduled_task_id": t.ID, "task_id": task.ID})
	writeJSON(w, http.StatusAccepted, task)
}

func (s *Server) scheduledTaskExecutions(w http.ResponseWriter, r *http.Request) {
	t, ok := s.teamScheduledTask(w, r, "resource.logs.read")
	if !ok {
		return
	}
	list, err := s.Store.TasksWithParam(r.Context(), "scheduled_task_id", t.ID, 50)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}
