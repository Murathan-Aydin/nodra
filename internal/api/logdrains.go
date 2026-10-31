package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func (s *Server) getLogDrain(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.read")
	if !ok {
		return
	}
	d, err := s.Store.LogDrain(r.Context(), sv.ID)
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	} else if err != nil {
		s.internal(w, err)
		return
	}
	raw, err := s.Box.Decrypt(d.ConfigEnc)
	if err != nil {
		s.internal(w, err)
		return
	}
	c, err := servers.DecodeLogDrain(raw)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"enabled": true, "type": d.Type, "config": c.Redacted(), "updated_at": d.UpdatedAt})
}

// putLogDrain enregistre le log drain puis (re)démarre Fluent Bit sur le serveur.
func (s *Server) putLogDrain(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.update")
	if !ok {
		return
	}
	var in struct {
		Type string `json:"type"`
		servers.LogDrainConfig
	}
	if !decode(w, r, &in) {
		return
	}
	if !slices.Contains(servers.LogDrainTypes, in.Type) {
		writeError(w, http.StatusUnprocessableEntity, "type : newrelic, axiom ou custom")
		return
	}
	ctx := r.Context()
	var cfg servers.LogDrainConfig
	if d, err := s.Store.LogDrain(ctx, sv.ID); err == nil {
		if raw, err := s.Box.Decrypt(d.ConfigEnc); err == nil {
			cfg, _ = servers.DecodeLogDrain(raw)
		}
	}
	cfg.Merge(in.LogDrainConfig)
	if err := cfg.Validate(in.Type); err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	b, _ := json.Marshal(cfg)
	enc, err := s.Box.Encrypt(string(b))
	if err != nil {
		s.internal(w, err)
		return
	}
	if err := s.Store.UpsertLogDrain(ctx, &store.LogDrain{ServerID: sv.ID, Type: in.Type, ConfigEnc: enc}); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "server.log_drain.updated", "server", sv.ID, map[string]string{"type": in.Type})
	task, err := s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: sv.TeamID, ServerID: sv.ID, Kind: servers.TaskLogDrainStart,
		CreatedBy: &userFrom(ctx).ID})
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"task": task, "config": cfg.Redacted()})
}

func (s *Server) deleteLogDrain(w http.ResponseWriter, r *http.Request) {
	sv, ok := s.teamServer(w, r, "server.update")
	if !ok {
		return
	}
	ctx := r.Context()
	if err := s.Store.DeleteLogDrain(ctx, sv.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "server.log_drain.deleted", "server", sv.ID, nil)
	task, err := s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: sv.TeamID, ServerID: sv.ID, Kind: servers.TaskLogDrainStop,
		CreatedBy: &userFrom(ctx).ID})
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, task)
}
