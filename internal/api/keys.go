package api

import (
	"net/http"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

func (s *Server) listKeys(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "key.read", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListPrivateKeys(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// createKey importe une clé privée ou, si private_key est vide, génère une paire ed25519.
func (s *Server) createKey(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "key.manage", rbac.Scope{}) {
		return
	}
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		PrivateKey  string `json:"private_key"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return
	}
	priv := strings.TrimSpace(in.PrivateKey) + "\n"
	if strings.TrimSpace(in.PrivateKey) == "" {
		var err error
		if priv, _, err = ssh.GenerateKey("nodra"); err != nil {
			s.internal(w, err)
			return
		}
	}
	pub, fp, err := ssh.ParseKey(priv)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	enc, err := s.Box.Encrypt(priv)
	if err != nil {
		s.internal(w, err)
		return
	}
	k, err := s.Store.CreatePrivateKey(r.Context(), &store.PrivateKey{TeamID: teamFrom(r.Context()).Team.ID, Name: in.Name,
		Description: strings.TrimSpace(in.Description), PrivateKeyEnc: enc, PublicKey: pub + " nodra", Fingerprint: fp})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "key.created", "private_key", k.ID, map[string]any{"name": k.Name, "fingerprint": fp, "generated": in.PrivateKey == ""})
	writeJSON(w, http.StatusCreated, k)
}

func (s *Server) teamKey(w http.ResponseWriter, r *http.Request, perm string) (*store.PrivateKey, bool) {
	if !s.authorize(w, r, perm, rbac.Scope{}) {
		return nil, false
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	k, err := s.Store.PrivateKeyByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return k, true
}

func (s *Server) getKey(w http.ResponseWriter, r *http.Request) {
	if k, ok := s.teamKey(w, r, "key.read"); ok {
		writeJSON(w, http.StatusOK, k)
	}
}

func (s *Server) updateKey(w http.ResponseWriter, r *http.Request) {
	k, ok := s.teamKey(w, r, "key.manage")
	if !ok {
		return
	}
	var in struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 100 {
		writeError(w, http.StatusUnprocessableEntity, "nom requis (100 caractères max.)")
		return
	}
	if err := s.Store.UpdatePrivateKey(r.Context(), k.TeamID, k.ID, in.Name, strings.TrimSpace(in.Description)); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "key.updated", "private_key", k.ID, map[string]any{"before": k.Name, "after": in.Name})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteKey(w http.ResponseWriter, r *http.Request) {
	k, ok := s.teamKey(w, r, "key.manage")
	if !ok {
		return
	}
	if err := s.Store.DeletePrivateKey(r.Context(), k.TeamID, k.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "key.deleted", "private_key", k.ID, map[string]any{"name": k.Name})
	w.WriteHeader(http.StatusNoContent)
}
