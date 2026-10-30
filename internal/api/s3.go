package api

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/backup"
	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

var bucketRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func (s *Server) listS3(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "storage.read", rbac.Scope{}) {
		return
	}
	list, err := s.Store.ListS3Storages(r.Context(), teamFrom(r.Context()).Team.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type s3Input struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Endpoint    *string `json:"endpoint"`
	Region      *string `json:"region"`
	Bucket      *string `json:"bucket"`
	PathPrefix  *string `json:"path_prefix"`
	AccessKey   *string `json:"access_key"`
	SecretKey   *string `json:"secret_key"`
}

func (s *Server) applyS3(in *s3Input, x *store.S3Storage) string {
	for _, f := range []struct {
		dst *string
		v   *string
	}{{&x.Name, in.Name}, {&x.Description, in.Description}, {&x.Endpoint, in.Endpoint}, {&x.Region, in.Region}, {&x.Bucket, in.Bucket},
		{&x.PathPrefix, in.PathPrefix}} {
		if f.v != nil {
			*f.dst = strings.TrimSpace(*f.v)
		}
	}
	for _, f := range []struct {
		dst *string
		v   *string
	}{{&x.AccessKeyEnc, in.AccessKey}, {&x.SecretKeyEnc, in.SecretKey}} {
		if f.v != nil && strings.TrimSpace(*f.v) != "" {
			enc, err := s.Box.Encrypt(strings.TrimSpace(*f.v))
			if err != nil {
				return "chiffrement impossible"
			}
			*f.dst = enc
		}
	}
	x.Endpoint = strings.TrimSuffix(x.Endpoint, "/")
	if x.Region == "" {
		x.Region = "us-east-1"
	}
	switch {
	case x.Name == "" || len(x.Name) > 100:
		return "nom requis (100 caractères max.)"
	case !validHTTPURL(x.Endpoint):
		return "endpoint : URL http(s), ex. https://s3.eu-west-3.amazonaws.com"
	case !bucketRe.MatchString(x.Bucket):
		return "nom de bucket invalide"
	case strings.Contains(x.PathPrefix, ".."):
		return "path_prefix invalide"
	case x.AccessKeyEnc == "" || x.SecretKeyEnc == "":
		return "access_key et secret_key requis"
	}
	return ""
}

// checkS3 vérifie l'accès au bucket et enregistre le résultat.
func (s *Server) checkS3(r *http.Request, x *store.S3Storage) {
	msg := ""
	dec, err := backup.Decrypt(s.Box, x)
	if err == nil {
		err = dec.Check(r.Context())
	}
	if err != nil {
		msg = err.Error()
	}
	x.IsUsable, x.LastError = err == nil, msg
	if err := s.Store.SetS3Check(r.Context(), x.ID, x.IsUsable, msg); err != nil {
		s.Log.Error("vérification S3", "err", err)
	}
}

func (s *Server) createS3(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r, "storage.manage", rbac.Scope{}) {
		return
	}
	var in s3Input
	if !decode(w, r, &in) {
		return
	}
	x := &store.S3Storage{TeamID: teamFrom(r.Context()).Team.ID}
	if msg := s.applyS3(&in, x); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	created, err := s.Store.CreateS3Storage(r.Context(), x)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.checkS3(r, created)
	s.auditTeam(r, "storage.created", "s3_storage", created.ID, map[string]any{"name": created.Name, "bucket": created.Bucket, "usable": created.IsUsable})
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) teamS3(w http.ResponseWriter, r *http.Request, perm string) (*store.S3Storage, bool) {
	if !s.authorize(w, r, perm, rbac.Scope{}) {
		return nil, false
	}
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	x, err := s.Store.S3StorageByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return x, true
}

func (s *Server) getS3(w http.ResponseWriter, r *http.Request) {
	if x, ok := s.teamS3(w, r, "storage.read"); ok {
		writeJSON(w, http.StatusOK, x)
	}
}

func (s *Server) updateS3(w http.ResponseWriter, r *http.Request) {
	x, ok := s.teamS3(w, r, "storage.manage")
	if !ok {
		return
	}
	var in s3Input
	if !decode(w, r, &in) {
		return
	}
	if msg := s.applyS3(&in, x); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if err := s.Store.UpdateS3Storage(r.Context(), x); err != nil {
		s.fail(w, err)
		return
	}
	s.checkS3(r, x)
	s.auditTeam(r, "storage.updated", "s3_storage", x.ID, map[string]any{"name": x.Name, "keys_changed": in.AccessKey != nil || in.SecretKey != nil})
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) checkS3Handler(w http.ResponseWriter, r *http.Request) {
	x, ok := s.teamS3(w, r, "storage.manage")
	if !ok {
		return
	}
	s.checkS3(r, x)
	writeJSON(w, http.StatusOK, x)
}

func (s *Server) deleteS3(w http.ResponseWriter, r *http.Request) {
	x, ok := s.teamS3(w, r, "storage.manage")
	if !ok {
		return
	}
	if err := s.Store.DeleteS3Storage(r.Context(), x.TeamID, x.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "storage.deleted", "s3_storage", x.ID, map[string]string{"name": x.Name})
	w.WriteHeader(http.StatusNoContent)
}
