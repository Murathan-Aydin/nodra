package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/envvars"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// envOp : opération sur les variables d'une ressource (valeurs chiffrées, y compris dans une demande d'approbation).
type envOp struct {
	Op  string        `json:"op"` // upsert | update | delete
	Var *store.EnvVar `json:"var"`
}

type envOpsPayload struct {
	ResourceID string  `json:"resource_id"`
	Ops        []envOp `json:"ops"`
}

// envVarWire sérialise la valeur chiffrée (masquée dans store.EnvVar) pour la stocker dans une demande d'approbation.
type envVarWire struct {
	*store.EnvVar
	ValueEnc string `json:"value_enc"`
}

func (o envOp) MarshalJSON() ([]byte, error) {
	var v *envVarWire
	if o.Var != nil {
		cp := *o.Var
		cp.Value = ""
		v = &envVarWire{&cp, o.Var.ValueEnc}
	}
	return json.Marshal(struct {
		Op  string      `json:"op"`
		Var *envVarWire `json:"var"`
	}{o.Op, v})
}

func (o *envOp) UnmarshalJSON(b []byte) error {
	var raw struct {
		Op  string `json:"op"`
		Var *struct {
			store.EnvVar
			ValueEnc string `json:"value_enc"`
		} `json:"var"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	o.Op = raw.Op
	if raw.Var != nil {
		v := raw.Var.EnvVar
		v.ValueEnc = raw.Var.ValueEnc
		o.Var = &v
	}
	return nil
}

func (s *Server) applyEnvOps(ctx context.Context, resourceID string, ops []envOp) error {
	for _, op := range ops {
		var err error
		switch op.Op {
		case "upsert":
			_, err = s.Store.UpsertEnvVar(ctx, op.Var)
		case "update":
			err = s.Store.UpdateEnvVar(ctx, op.Var)
		case "delete":
			err = s.Store.DeleteEnvVar(ctx, resourceID, op.Var.ID)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) execEnvWrite(ctx context.Context, req *store.ApprovalRequest) error {
	var p envOpsPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	return s.applyEnvOps(ctx, p.ResourceID, p.Ops)
}

// writeEnvs applique des opérations, ou crée une demande d'approbation si une règle couvre resource.env.write.
func (s *Server) writeEnvs(w http.ResponseWriter, r *http.Request, resourceID, summary string, ops []envOp) {
	req, err := s.gate(r, "resource.env.write", resourceScope(resourceID), summary, envOpsPayload{resourceID, ops})
	if err != nil {
		s.fail(w, err)
		return
	}
	if req != nil {
		respondPending(w, req)
		return
	}
	if err := s.applyEnvOps(r.Context(), resourceID, ops); err != nil {
		s.fail(w, err)
		return
	}
	keys := make([]string, len(ops))
	for i, op := range ops {
		keys[i] = op.Op + " " + op.Var.Key
	}
	s.auditTeam(r, "resource.env.updated", "resource", resourceID, map[string]any{"changes": keys})
	list, err := s.listEnvValues(r.Context(), resourceID)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) listEnvValues(ctx context.Context, resourceID string) ([]*store.EnvVar, error) {
	list, err := s.Store.ListEnvVars(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	for _, v := range list {
		if v.IsSecret {
			continue
		}
		if v.Value, err = s.Box.Decrypt(v.ValueEnc); err != nil {
			return nil, err
		}
	}
	return list, nil
}

type envInput struct {
	Key         string  `json:"key"`
	Value       *string `json:"value"`
	IsBuildtime *bool   `json:"is_buildtime"`
	IsRuntime   *bool   `json:"is_runtime"`
	IsPreview   bool    `json:"is_preview"`
	IsLiteral   *bool   `json:"is_literal"`
	IsMultiline *bool   `json:"is_multiline"`
	IsSecret    *bool   `json:"is_secret"`
	Comment     *string `json:"comment"`
}

func (in *envInput) applyTo(s *Server, v *store.EnvVar) string {
	if in.Value != nil {
		if len(*in.Value) > 64<<10 {
			return "valeur trop longue (64 Ko max.)"
		}
		enc, err := s.Box.Encrypt(*in.Value)
		if err != nil {
			return "chiffrement impossible"
		}
		v.ValueEnc = enc
	}
	for _, f := range []struct {
		dst *bool
		v   *bool
	}{{&v.IsBuildtime, in.IsBuildtime}, {&v.IsRuntime, in.IsRuntime}, {&v.IsLiteral, in.IsLiteral}, {&v.IsMultiline, in.IsMultiline}} {
		if f.v != nil {
			*f.dst = *f.v
		}
	}
	if in.IsSecret != nil {
		if v.IsSecret && !*in.IsSecret && in.Value == nil {
			return "un secret ne peut redevenir visible qu'en redéfinissant sa valeur"
		}
		v.IsSecret = *in.IsSecret
	}
	if in.Comment != nil {
		v.Comment = truncate(strings.TrimSpace(*in.Comment), 500)
	}
	if !v.IsBuildtime && !v.IsRuntime {
		return "la variable doit être disponible au build, à l'exécution, ou les deux"
	}
	return ""
}

// --- Handlers génériques (paramétrés par le chargeur de ressource) ---

type resourceLoader func(w http.ResponseWriter, r *http.Request, perm string) (id, typ, name string, ok bool)

func (s *Server) appLoader(w http.ResponseWriter, r *http.Request, perm string) (string, string, string, bool) {
	a, ok := s.teamApp(w, r, perm)
	if !ok {
		return "", "", "", false
	}
	return a.ID, "application", a.Name, true
}

func (s *Server) listEnvs(load resourceLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _, _, ok := load(w, r, "resource.env.read")
		if !ok {
			return
		}
		list, err := s.listEnvValues(r.Context(), id)
		if err != nil {
			s.internal(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

func (s *Server) createEnv(load resourceLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, typ, name, ok := load(w, r, "resource.env.write")
		if !ok {
			return
		}
		var in envInput
		if !decode(w, r, &in) {
			return
		}
		if !envvars.KeyRe.MatchString(in.Key) {
			writeError(w, http.StatusUnprocessableEntity, "clé invalide (lettres, chiffres et _, sans commencer par un chiffre)")
			return
		}
		if in.Value == nil {
			writeError(w, http.StatusUnprocessableEntity, "valeur requise")
			return
		}
		v := &store.EnvVar{ResourceType: typ, ResourceID: id, Key: in.Key, IsPreview: in.IsPreview, IsRuntime: true, IsBuildtime: true}
		if msg := in.applyTo(s, v); msg != "" {
			writeError(w, http.StatusUnprocessableEntity, msg)
			return
		}
		s.writeEnvs(w, r, id, "Variable "+in.Key+" de "+name, []envOp{{"upsert", v}})
	}
}

func (s *Server) updateEnv(load resourceLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _, name, ok := load(w, r, "resource.env.write")
		if !ok {
			return
		}
		envID, ok := idParam(w, r, "env")
		if !ok {
			return
		}
		v, err := s.Store.EnvVarByID(r.Context(), id, envID)
		if err != nil {
			s.fail(w, err)
			return
		}
		var in envInput
		if !decode(w, r, &in) {
			return
		}
		if msg := in.applyTo(s, v); msg != "" {
			writeError(w, http.StatusUnprocessableEntity, msg)
			return
		}
		s.writeEnvs(w, r, id, "Variable "+v.Key+" de "+name, []envOp{{"update", v}})
	}
}

func (s *Server) deleteEnv(load resourceLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _, name, ok := load(w, r, "resource.env.write")
		if !ok {
			return
		}
		envID, ok := idParam(w, r, "env")
		if !ok {
			return
		}
		v, err := s.Store.EnvVarByID(r.Context(), id, envID)
		if err != nil {
			s.fail(w, err)
			return
		}
		s.writeEnvs(w, r, id, "Suppression de "+v.Key+" de "+name, []envOp{{"delete", v}})
	}
}

// bulkEnvs importe un contenu .env (crée ou remplace chaque clé).
func (s *Server) bulkEnvs(load resourceLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, typ, name, ok := load(w, r, "resource.env.write")
		if !ok {
			return
		}
		var in struct {
			Content     string `json:"content"`
			IsPreview   bool   `json:"is_preview"`
			IsBuildtime *bool  `json:"is_buildtime"`
		}
		if !decode(w, r, &in) {
			return
		}
		pairs, err := envvars.ParseDotenv(in.Content)
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		build := true
		if in.IsBuildtime != nil {
			build = *in.IsBuildtime
		}
		ops := make([]envOp, 0, len(pairs))
		for _, p := range pairs {
			enc, err := s.Box.Encrypt(p.Value)
			if err != nil {
				s.internal(w, err)
				return
			}
			ops = append(ops, envOp{"upsert", &store.EnvVar{ResourceType: typ, ResourceID: id, Key: p.Key, ValueEnc: enc,
				IsPreview: in.IsPreview, IsBuildtime: build, IsRuntime: true, IsMultiline: p.Multiline}})
		}
		s.writeEnvs(w, r, id, "Import de variables dans "+name, ops)
	}
}

// --- Stockages ---

func (s *Server) listStorages(load resourceLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _, _, ok := load(w, r, "resource.read")
		if !ok {
			return
		}
		list, err := s.Store.ListStorages(r.Context(), id)
		if err != nil {
			s.internal(w, err)
			return
		}
		writeJSON(w, http.StatusOK, list)
	}
}

func cleanAbs(p string) (string, bool) {
	if !strings.HasPrefix(p, "/") || strings.Contains(p, "..") || strings.ContainsAny(p, "\x00\n\r:,") || len(p) > 4096 {
		return "", false
	}
	return path.Clean(p), true
}

func (s *Server) createStorage(load resourceLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, typ, _, ok := load(w, r, "resource.update")
		if !ok {
			return
		}
		var in store.Storage
		if !decode(w, r, &in) {
			return
		}
		mount, ok := cleanAbs(in.MountPath)
		if !ok || mount == "/" {
			writeError(w, http.StatusUnprocessableEntity, "mount_path : chemin absolu dans le conteneur")
			return
		}
		st := &store.Storage{ResourceType: typ, ResourceID: id, Kind: in.Kind, MountPath: mount}
		switch in.Kind {
		case "volume":
			st.Name = strings.TrimSpace(in.Name)
			if st.Name == "" {
				st.Name = strings.ReplaceAll(id, "-", "")[:12] + strings.ReplaceAll(mount, "/", "-")
			}
			if !volumeNameRe.MatchString(st.Name) {
				writeError(w, http.StatusUnprocessableEntity, "nom de volume invalide")
				return
			}
		case "bind":
			host, ok := cleanAbs(in.HostPath)
			if !ok || host == "/" {
				writeError(w, http.StatusUnprocessableEntity, "host_path : chemin absolu sur le serveur")
				return
			}
			st.HostPath = host
		case "file":
			if len(in.Content) > 1<<20 {
				writeError(w, http.StatusUnprocessableEntity, "contenu trop long (1 Mo max.)")
				return
			}
			st.Content = in.Content
		default:
			writeError(w, http.StatusUnprocessableEntity, "kind : volume, bind ou file")
			return
		}
		created, err := s.Store.CreateStorage(r.Context(), st)
		if err != nil {
			s.fail(w, err)
			return
		}
		s.auditTeam(r, "resource.storage.created", "resource", id, map[string]string{"kind": st.Kind, "mount_path": mount})
		writeJSON(w, http.StatusCreated, created)
	}
}

func (s *Server) updateStorage(load resourceLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _, _, ok := load(w, r, "resource.update")
		if !ok {
			return
		}
		sid, ok := idParam(w, r, "storage")
		if !ok {
			return
		}
		var in struct {
			Content string `json:"content"`
		}
		if !decode(w, r, &in) {
			return
		}
		if len(in.Content) > 1<<20 {
			writeError(w, http.StatusUnprocessableEntity, "contenu trop long (1 Mo max.)")
			return
		}
		if err := s.Store.UpdateStorageContent(r.Context(), id, sid, in.Content); err != nil {
			s.fail(w, err)
			return
		}
		s.auditTeam(r, "resource.storage.updated", "resource", id, map[string]string{"storage_id": sid})
		w.WriteHeader(http.StatusNoContent)
	}
}

func (s *Server) deleteStorage(load resourceLoader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, _, _, ok := load(w, r, "resource.update")
		if !ok {
			return
		}
		sid, ok := idParam(w, r, "storage")
		if !ok {
			return
		}
		if err := s.Store.DeleteStorage(r.Context(), id, sid); err != nil {
			s.fail(w, err)
			return
		}
		s.auditTeam(r, "resource.storage.deleted", "resource", id, map[string]string{"storage_id": sid})
		w.WriteHeader(http.StatusNoContent)
	}
}
