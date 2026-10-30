package api

import (
	"context"
	"encoding/json"
	"net/http"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/backup"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// resourceType vérifie qu'une ressource appartient à l'équipe courante et renvoie son type.
func (s *Server) resourceType(ctx context.Context, id string) (string, error) {
	if _, err := s.Store.DatabaseByID(ctx, teamFrom(ctx).Team.ID, id); err == nil {
		return "database", nil
	}
	if _, err := s.Store.ApplicationByID(ctx, teamFrom(ctx).Team.ID, id); err == nil {
		return "application", nil
	}
	if _, err := s.Store.ServiceByID(ctx, teamFrom(ctx).Team.ID, id); err == nil {
		return "service", nil
	}
	return "", store.ErrNotFound
}

// volumeAllowed : le volume appartient-il à la ressource (stockage déclaré ou volume préfixé par son id) ?
func (s *Server) volumeAllowed(ctx context.Context, resourceID, volume string) bool {
	if !volumeNameRe.MatchString(volume) {
		return false
	}
	if strings.HasPrefix(volume, resourceID) || strings.HasPrefix(volume, strings.ReplaceAll(resourceID, "-", "")[:12]) {
		return true
	}
	list, err := s.Store.ListStorages(ctx, resourceID)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(list, func(st *store.Storage) bool { return st.Kind == "volume" && st.Name == volume })
}

func (s *Server) listBackupSchedules(w http.ResponseWriter, r *http.Request) {
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
	list, err := s.Store.ListBackupSchedules(r.Context(), id)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type scheduleInput struct {
	Kind          string  `json:"kind"`
	ResourceID    string  `json:"resource_id"`
	VolumeName    *string `json:"volume_name"`
	Databases     *string `json:"databases"`
	DumpAll       *bool   `json:"dump_all"`
	Frequency     *string `json:"frequency"`
	Enabled       *bool   `json:"enabled"`
	KeepLocal     *int    `json:"keep_local"`
	KeepLocalDays *int    `json:"keep_local_days"`
	LocalBackup   *bool   `json:"local_backup"`
	S3StorageID   *string `json:"s3_storage_id"`
	KeepS3        *int    `json:"keep_s3"`
	KeepS3Days    *int    `json:"keep_s3_days"`
}

func (s *Server) applySchedule(ctx context.Context, in *scheduleInput, b *store.BackupSchedule) string {
	if in.VolumeName != nil {
		b.VolumeName = strings.TrimSpace(*in.VolumeName)
	}
	if in.Databases != nil {
		b.Databases = strings.TrimSpace(*in.Databases)
	}
	if in.Frequency != nil {
		b.Frequency = strings.TrimSpace(*in.Frequency)
	}
	for _, f := range []struct {
		dst *bool
		v   *bool
	}{{&b.DumpAll, in.DumpAll}, {&b.Enabled, in.Enabled}, {&b.LocalBackup, in.LocalBackup}} {
		if f.v != nil {
			*f.dst = *f.v
		}
	}
	for _, f := range []struct {
		dst *int
		v   *int
	}{{&b.KeepLocal, in.KeepLocal}, {&b.KeepLocalDays, in.KeepLocalDays}, {&b.KeepS3, in.KeepS3}, {&b.KeepS3Days, in.KeepS3Days}} {
		if f.v != nil {
			*f.dst = *f.v
		}
	}
	if in.S3StorageID != nil {
		b.S3StorageID = nilIfEmpty(*in.S3StorageID)
	}
	if err := backup.ParseCron(b.Frequency); err != nil {
		return "frequency : expression cron invalide (ex. 0 3 * * * ou @daily)"
	}
	if b.KeepLocal < 0 || b.KeepLocalDays < 0 || b.KeepS3 < 0 || b.KeepS3Days < 0 {
		return "valeurs de rétention négatives"
	}
	if b.S3StorageID != nil {
		if !isUUID(*b.S3StorageID) {
			return "stockage S3 inconnu"
		}
		if _, err := s.Store.S3StorageByID(ctx, b.TeamID, *b.S3StorageID); err != nil {
			return "stockage S3 inconnu"
		}
	} else if !b.LocalBackup {
		return "sans stockage S3, la sauvegarde doit être conservée sur le serveur"
	}
	switch b.Kind {
	case "database":
		db, err := s.Store.DatabaseByID(ctx, b.TeamID, b.ResourceID)
		if err != nil {
			return "la ressource n'est pas une base de données"
		}
		if ok, _ := backup.Supported(db.Type); !ok {
			return "sauvegarde non prise en charge pour " + db.Type
		}
		for _, n := range strings.Split(b.Databases, ",") {
			if n = strings.TrimSpace(n); n != "" && !backup.ValidDBName(n) {
				return "nom de base invalide : " + n
			}
		}
	case "volume":
		if !s.volumeAllowed(ctx, b.ResourceID, b.VolumeName) {
			return "volume inconnu pour cette ressource"
		}
	default:
		return "kind : database ou volume"
	}
	return ""
}

func (s *Server) createBackupSchedule(w http.ResponseWriter, r *http.Request) {
	var in scheduleInput
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
	if !s.authorize(w, r, "resource.backup.manage", resourceScope(in.ResourceID)) {
		return
	}
	b := &store.BackupSchedule{TeamID: teamFrom(ctx).Team.ID, Kind: in.Kind, ResourceID: in.ResourceID, Frequency: "0 0 * * *", Enabled: true,
		KeepLocal: 7, LocalBackup: true, KeepS3: 30}
	if msg := s.applySchedule(ctx, &in, b); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	created, err := s.Store.CreateBackupSchedule(ctx, b)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "backup.schedule.created", "resource", b.ResourceID, created)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) teamSchedule(w http.ResponseWriter, r *http.Request, perm string) (*store.BackupSchedule, bool) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	b, err := s.Store.BackupScheduleByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return b, s.authorize(w, r, perm, resourceScope(b.ResourceID))
}

func (s *Server) updateBackupSchedule(w http.ResponseWriter, r *http.Request) {
	b, ok := s.teamSchedule(w, r, "resource.backup.manage")
	if !ok {
		return
	}
	var in scheduleInput
	if !decode(w, r, &in) {
		return
	}
	if msg := s.applySchedule(r.Context(), &in, b); msg != "" {
		writeError(w, http.StatusUnprocessableEntity, msg)
		return
	}
	if err := s.Store.UpdateBackupSchedule(r.Context(), b); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "backup.schedule.updated", "resource", b.ResourceID, b)
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) deleteBackupSchedule(w http.ResponseWriter, r *http.Request) {
	b, ok := s.teamSchedule(w, r, "resource.backup.manage")
	if !ok {
		return
	}
	if err := s.Store.DeleteBackupSchedule(r.Context(), b.TeamID, b.ID); err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "backup.schedule.deleted", "resource", b.ResourceID, map[string]string{"schedule_id": b.ID})
	w.WriteHeader(http.StatusNoContent)
}

// runBackupNow lance immédiatement une sauvegarde planifiée.
func (s *Server) runBackupNow(w http.ResponseWriter, r *http.Request) {
	b, ok := s.teamSchedule(w, r, "resource.backup.manage")
	if !ok {
		return
	}
	ctx := r.Context()
	if busy, err := s.Store.HasActiveTaskWithParam(ctx, "schedule_id", b.ID); err != nil {
		s.internal(w, err)
		return
	} else if busy {
		writeError(w, http.StatusConflict, "une sauvegarde est déjà en cours")
		return
	}
	uid := userFrom(ctx).ID
	task, err := s.Jobs.EnqueueBackup(ctx, b, &uid)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.auditTeam(r, "backup.run", "resource", b.ResourceID, map[string]string{"schedule_id": b.ID, "task_id": task.ID})
	writeJSON(w, http.StatusAccepted, task)
}

func (s *Server) listBackupExecutions(w http.ResponseWriter, r *http.Request) {
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
	list, err := s.Store.ListBackupExecutions(r.Context(), id, 100)
	if err != nil {
		s.internal(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) teamExecution(w http.ResponseWriter, r *http.Request, perm string) (*store.BackupExecution, bool) {
	id, ok := idParam(w, r, "id")
	if !ok {
		return nil, false
	}
	e, err := s.Store.BackupExecutionByID(r.Context(), teamFrom(r.Context()).Team.ID, id)
	if err != nil {
		s.fail(w, err)
		return nil, false
	}
	return e, s.authorize(w, r, perm, resourceScope(e.ResourceID))
}

type restorePayload struct {
	TeamID      string  `json:"team_id"`
	ExecutionID string  `json:"execution_id"`
	ResourceID  string  `json:"resource_id"`
	CreatedBy   *string `json:"created_by,omitempty"`
}

func (s *Server) enqueueRestore(ctx context.Context, e *store.BackupExecution, by *string) (*store.ServerTask, error) {
	params, _ := json.Marshal(backup.RestoreParams{ExecutionID: e.ID, ResourceID: e.ResourceID})
	return s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: e.TeamID, ServerID: e.ServerID, Kind: backup.TaskBackupRestore,
		Params: params, CreatedBy: by})
}

func (s *Server) execRestore(ctx context.Context, req *store.ApprovalRequest) error {
	var p restorePayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	if p.ExecutionID == "" {
		return s.execImport(ctx, req)
	}
	e, err := s.Store.BackupExecutionByID(ctx, p.TeamID, p.ExecutionID)
	if err != nil {
		return err
	}
	_, err = s.enqueueRestore(ctx, e, p.CreatedBy)
	return err
}

func (s *Server) restoreBackup(w http.ResponseWriter, r *http.Request) {
	e, ok := s.teamExecution(w, r, "resource.backup.restore")
	if !ok {
		return
	}
	if e.Status != "success" || (e.LocalDeleted && (e.S3Key == "" || e.S3Deleted)) {
		writeError(w, http.StatusConflict, "cette sauvegarde n'est pas restaurable")
		return
	}
	ctx := r.Context()
	uid := userFrom(ctx).ID
	pending, err := s.gate(r, "resource.backup.restore", resourceScope(e.ResourceID), "Restauration de la sauvegarde "+path.Base(e.Filename),
		restorePayload{e.TeamID, e.ID, e.ResourceID, &uid})
	if err != nil {
		s.fail(w, err)
		return
	}
	if pending != nil {
		respondPending(w, pending)
		return
	}
	task, err := s.enqueueRestore(ctx, e, &uid)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "backup.restore", "resource", e.ResourceID, map[string]string{"execution_id": e.ID, "task_id": task.ID})
	writeJSON(w, http.StatusAccepted, task)
}

// downloadBackup : lien S3 temporaire si disponible, sinon contenu du fichier lu sur le serveur.
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	e, ok := s.teamExecution(w, r, "resource.backup.manage")
	if !ok {
		return
	}
	ctx := r.Context()
	if e.Status != "success" {
		writeError(w, http.StatusConflict, "sauvegarde en échec ou en cours")
		return
	}
	s.auditTeam(r, "backup.downloaded", "resource", e.ResourceID, map[string]string{"execution_id": e.ID})
	if e.S3Key != "" && !e.S3Deleted && e.S3StorageID != nil {
		if st, err := s.Store.S3StorageByIDAny(ctx, *e.S3StorageID); err == nil {
			dec, err := backup.Decrypt(s.Box, st)
			if err == nil {
				if u, err := dec.Presign(ctx, e.S3Key, 15*time.Minute); err == nil {
					writeJSON(w, http.StatusOK, map[string]string{"url": u})
					return
				}
			}
		}
	}
	if e.LocalDeleted {
		writeError(w, http.StatusGone, "le fichier n'est plus disponible")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+path.Base(e.Filename)+`"`)
	if err := s.Backups.Stream(ctx, s.Targets, e.ServerID, e.Filename, w); err != nil {
		s.Log.Error("téléchargement de sauvegarde", "execution", e.ID, "err", err)
	}
}

func (s *Server) deleteBackupExecution(w http.ResponseWriter, r *http.Request) {
	e, ok := s.teamExecution(w, r, "resource.backup.manage")
	if !ok {
		return
	}
	if err := s.Backups.Remove(r.Context(), s.Targets, e); err != nil {
		writeError(w, http.StatusBadGateway, "suppression impossible : "+err.Error())
		return
	}
	s.auditTeam(r, "backup.deleted", "resource", e.ResourceID, map[string]string{"execution_id": e.ID})
	w.WriteHeader(http.StatusNoContent)
}

// --- Import d'un dump ---

type importPayload struct {
	TeamID     string  `json:"team_id"`
	DatabaseID string  `json:"database_id"`
	File       string  `json:"file"`
	Database   string  `json:"database,omitempty"`
	CreatedBy  *string `json:"created_by,omitempty"`
}

func (s *Server) enqueueImport(ctx context.Context, p importPayload, serverID string) (*store.ServerTask, error) {
	params, _ := json.Marshal(backup.ImportParams{DatabaseID: p.DatabaseID, File: p.File, Database: p.Database})
	return s.Jobs.EnqueueServerTask(ctx, &store.ServerTask{TeamID: p.TeamID, ServerID: serverID, Kind: backup.TaskDatabaseImport,
		Params: params, CreatedBy: p.CreatedBy})
}

func (s *Server) execImport(ctx context.Context, req *store.ApprovalRequest) error {
	var p importPayload
	if err := json.Unmarshal(req.Payload, &p); err != nil {
		return err
	}
	loc, err := s.Store.ResourceLocation(ctx, p.DatabaseID)
	if err != nil {
		return err
	}
	_, err = s.enqueueImport(ctx, p, loc.ServerID)
	return err
}

var importExts = []string{".dump", ".sql", ".sql.gz", ".archive", ".archive.gz", ".gz"}

// importDatabase reçoit un dump (corps brut de la requête, nom dans ?filename=) et le restaure.
func (s *Server) importDatabase(w http.ResponseWriter, r *http.Request) {
	d, ok := s.teamDatabase(w, r, "resource.backup.restore")
	if !ok {
		return
	}
	ctx := r.Context()
	if _, restorable := backup.Supported(d.Type); !restorable {
		writeError(w, http.StatusUnprocessableEntity, "import non pris en charge pour "+d.Type)
		return
	}
	name := path.Base(r.URL.Query().Get("filename"))
	if !slices.ContainsFunc(importExts, func(ext string) bool { return strings.HasSuffix(name, ext) }) {
		writeError(w, http.StatusUnprocessableEntity, "filename requis : .dump, .sql, .sql.gz, .archive ou .archive.gz")
		return
	}
	dbName := r.URL.Query().Get("database")
	if dbName != "" && !backup.ValidDBName(dbName) {
		writeError(w, http.StatusUnprocessableEntity, "nom de base invalide")
		return
	}
	loc, err := s.Store.ResourceLocation(ctx, d.ID)
	if err != nil {
		s.internal(w, err)
		return
	}
	dest := s.Backups.ImportPath(d.ID, name)
	size, err := s.Backups.Upload(ctx, s.Targets, loc.ServerID, dest, http.MaxBytesReader(w, r.Body, 20<<30))
	if err != nil {
		writeError(w, http.StatusBadGateway, "envoi vers le serveur impossible : "+err.Error())
		return
	}
	uid := userFrom(ctx).ID
	p := importPayload{d.TeamID, d.ID, dest, dbName, &uid}
	pending, err := s.gate(r, "resource.backup.restore", resourceScope(d.ID), "Import de "+name+" dans "+d.Name, p)
	if err != nil {
		s.fail(w, err)
		return
	}
	if pending != nil {
		respondPending(w, pending)
		return
	}
	task, err := s.enqueueImport(ctx, p, loc.ServerID)
	if err != nil {
		s.internal(w, err)
		return
	}
	s.auditTeam(r, "database.import", "database", d.ID, map[string]any{"file": name, "size": size, "task_id": task.ID})
	writeJSON(w, http.StatusAccepted, task)
}
