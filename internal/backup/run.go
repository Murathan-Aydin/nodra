package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/crypto"
	"github.com/Murathan-Aydin/nodra/internal/servers"
	"github.com/Murathan-Aydin/nodra/internal/ssh"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

const (
	TaskBackupRun      = "backup_run"
	TaskBackupRestore  = "backup_restore"
	TaskDatabaseImport = "database_import"
)

type RunParams struct {
	ScheduleID string `json:"schedule_id"`
}

type RestoreParams struct {
	ExecutionID string `json:"execution_id"`
	ResourceID  string `json:"resource_id"`
}

type ImportParams struct {
	DatabaseID string `json:"database_id"`
	File       string `json:"file"`
	Database   string `json:"database,omitempty"`
}

type Service struct {
	Store *store.Store
	Box   *crypto.Box
	SSH   *ssh.Pool
	Log   *slog.Logger
	Dir   string // dossier des sauvegardes (Dir par défaut)
	// OnResult est appelé après chaque sauvegarde (notifications).
	OnResult func(ctx context.Context, e *store.BackupExecution)
}

func (s *Service) dir() string {
	if s.Dir != "" {
		return s.Dir
	}
	return Dir
}

type run struct {
	s   *Service
	t   ssh.Target
	out io.Writer
}

func (r *run) sh(ctx context.Context, label, script string) error {
	if label != "" {
		fmt.Fprintf(r.out, "$ %s\n", label)
	}
	code, err := r.s.SSH.RunInput(ctx, r.t, "sh -s", strings.NewReader("set -e\nset -o pipefail 2>/dev/null || true\n"+script), r.out, r.out)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("%s : échec (code %d)", label, code)
	}
	return nil
}

func (r *run) output(ctx context.Context, script string) (string, error) {
	var out bytes.Buffer
	code, err := r.s.SSH.RunInput(ctx, r.t, "sh -s", strings.NewReader(script), &out, io.Discard)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("code %d", code)
	}
	return strings.TrimSpace(out.String()), nil
}

// Tasks : tâches serveur des sauvegardes, à brancher dans servers.Service.Tasks.
func (s *Service) Tasks() map[string]servers.TaskFunc {
	return map[string]servers.TaskFunc{
		TaskBackupRun: func(ctx context.Context, task *store.ServerTask, sv *store.Server, t ssh.Target, out io.Writer) error {
			var p RunParams
			if err := json.Unmarshal(task.Params, &p); err != nil {
				return err
			}
			return s.runSchedule(ctx, &run{s, t, out}, sv, p.ScheduleID)
		},
		TaskBackupRestore: func(ctx context.Context, task *store.ServerTask, sv *store.Server, t ssh.Target, out io.Writer) error {
			var p RestoreParams
			if err := json.Unmarshal(task.Params, &p); err != nil {
				return err
			}
			return s.restore(ctx, &run{s, t, out}, sv, p.ExecutionID)
		},
		TaskDatabaseImport: func(ctx context.Context, task *store.ServerTask, sv *store.Server, t ssh.Target, out io.Writer) error {
			var p ImportParams
			if err := json.Unmarshal(task.Params, &p); err != nil {
				return err
			}
			return s.importDump(ctx, &run{s, t, out}, p)
		},
	}
}

var unsafeChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func (s *Service) runSchedule(ctx context.Context, r *run, sv *store.Server, scheduleID string) error {
	b, err := s.Store.BackupScheduleByIDAny(ctx, scheduleID)
	if err != nil {
		return err
	}
	var s3 *S3
	if b.S3StorageID != nil {
		st, err := s.Store.S3StorageByIDAny(ctx, *b.S3StorageID)
		if err != nil {
			return fmt.Errorf("stockage S3 : %w", err)
		}
		dec, err := Decrypt(s.Box, st)
		if err != nil {
			return err
		}
		s3 = &dec
	}
	type job struct{ name, script, file string }
	var jobs []job
	stamp := time.Now().UTC().Format("20060102-150405")
	base := s.dir() + "/" + b.ResourceID + "/"
	switch b.Kind {
	case "database":
		db, err := s.Store.DatabaseByIDAny(ctx, b.ResourceID)
		if err != nil {
			return err
		}
		names := []string{""}
		if !b.DumpAll && strings.TrimSpace(b.Databases) != "" {
			names = nil
			for _, n := range strings.Split(b.Databases, ",") {
				if n = strings.TrimSpace(n); n != "" {
					names = append(names, n)
				}
			}
		}
		for _, n := range names {
			label := n
			if label == "" {
				label = map[bool]string{true: "all", false: "default"}[b.DumpAll]
			}
			file := base + unsafeChars.ReplaceAllString(label, "_") + "-" + stamp + Extension(db.Type, b.DumpAll)
			script, err := DumpScript(db.Type, db.ID, n, b.DumpAll, file)
			if err != nil {
				return err
			}
			jobs = append(jobs, job{n, script, file})
		}
	case "volume":
		file := base + unsafeChars.ReplaceAllString(b.VolumeName, "_") + "-" + stamp + ".tar.gz"
		jobs = append(jobs, job{b.VolumeName, VolumeBackupScript(b.VolumeName, file), file})
	}
	var failed []string
	for _, j := range jobs {
		e, err := s.Store.CreateBackupExecution(ctx, &store.BackupExecution{ScheduleID: &b.ID, TeamID: b.TeamID, ResourceID: b.ResourceID,
			ServerID: sv.ID, DatabaseName: j.name, Filename: j.file, S3StorageID: b.S3StorageID})
		if err != nil {
			return err
		}
		if err := s.execute(ctx, r, b, s3, e, j.script); err != nil {
			failed = append(failed, fmt.Sprintf("%s : %s", path.Base(j.file), err))
		}
		if s.OnResult != nil {
			s.OnResult(context.WithoutCancel(ctx), e)
		}
	}
	s.retention(ctx, r, b, s3)
	if len(failed) > 0 {
		return errors.New(strings.Join(failed, " ; "))
	}
	return nil
}

func (s *Service) execute(ctx context.Context, r *run, b *store.BackupSchedule, s3 *S3, e *store.BackupExecution, script string) error {
	err := func() error {
		if err := r.sh(ctx, "sauvegarde "+path.Base(e.Filename), script); err != nil {
			return err
		}
		size, err := r.output(ctx, "wc -c < "+ssh.Quote(e.Filename)+"\n")
		if err != nil {
			return err
		}
		e.Size, _ = strconv.ParseInt(strings.TrimSpace(size), 10, 64)
		fmt.Fprintf(r.out, "Sauvegarde de %d octets.\n", e.Size)
		if s3 != nil {
			key := s3.Key(b.TeamID, b.ResourceID, path.Base(e.Filename))
			if err := r.sh(ctx, "envoi vers S3 ("+s3.Bucket+"/"+key+")", s3.UploadScript(e.Filename, key)); err != nil {
				return err
			}
			e.S3Key = key
			if !b.LocalBackup {
				if err := r.sh(ctx, "", "rm -f "+ssh.Quote(e.Filename)+"\n"); err != nil {
					return err
				}
				e.LocalDeleted = true
			}
		}
		return nil
	}()
	e.Status = "success"
	if err != nil {
		e.Status, e.Message = "failed", err.Error()
		_, _ = r.output(context.WithoutCancel(ctx), "rm -f "+ssh.Quote(e.Filename)+"\n")
		e.LocalDeleted = true
	}
	if ferr := s.Store.FinishBackupExecution(context.WithoutCancel(ctx), e); ferr != nil {
		return ferr
	}
	return err
}

// retention supprime les sauvegardes locales et S3 au-delà des limites de la planification.
func (s *Service) retention(ctx context.Context, r *run, b *store.BackupSchedule, s3 *S3) {
	names := map[string]bool{}
	all, err := s.Store.ListBackupExecutions(ctx, b.ResourceID, 10000)
	if err != nil {
		return
	}
	for _, e := range all {
		names[e.DatabaseName] = true
	}
	now := time.Now()
	for name := range names {
		list, err := s.Store.ScheduleExecutions(ctx, b.ID, name)
		if err != nil {
			continue
		}
		var local, remote []*store.BackupExecution
		for _, e := range list {
			if !e.LocalDeleted {
				local = append(local, e)
			}
			if e.S3Key != "" && !e.S3Deleted {
				remote = append(remote, e)
			}
		}
		for _, e := range Expired(local, b.KeepLocal, b.KeepLocalDays, now) {
			if _, err := r.output(ctx, "rm -f "+ssh.Quote(e.Filename)+"\n"); err == nil {
				fmt.Fprintf(r.out, "Rétention : suppression locale de %s.\n", path.Base(e.Filename))
				_ = s.Store.MarkBackupDeleted(ctx, e.ID, true, false)
			}
		}
		if s3 == nil {
			continue
		}
		for _, e := range Expired(remote, b.KeepS3, b.KeepS3Days, now) {
			if err := s3.Remove(ctx, e.S3Key); err == nil {
				fmt.Fprintf(r.out, "Rétention : suppression S3 de %s.\n", e.S3Key)
				_ = s.Store.MarkBackupDeleted(ctx, e.ID, false, true)
			} else {
				fmt.Fprintf(r.out, "Rétention S3 impossible pour %s : %s\n", e.S3Key, err)
			}
		}
	}
}

func (s *Service) restore(ctx context.Context, r *run, sv *store.Server, executionID string) error {
	e, err := s.Store.BackupExecutionByIDAny(ctx, executionID)
	if err != nil {
		return err
	}
	if e.Status != "success" {
		return errors.New("seule une sauvegarde réussie peut être restaurée")
	}
	file := e.Filename
	temp := false
	if e.LocalDeleted {
		if e.S3Key == "" || e.S3Deleted || e.S3StorageID == nil {
			return errors.New("le fichier de sauvegarde n'existe plus (ni sur le serveur, ni sur S3)")
		}
		st, err := s.Store.S3StorageByIDAny(ctx, *e.S3StorageID)
		if err != nil {
			return err
		}
		dec, err := Decrypt(s.Box, st)
		if err != nil {
			return err
		}
		file = s.dir() + "/" + e.ResourceID + "/restore-" + e.ID + "-" + path.Base(e.Filename)
		temp = true
		if err := r.sh(ctx, "téléchargement depuis S3", dec.DownloadScript(e.S3Key, file)); err != nil {
			return err
		}
	}
	defer func() {
		if temp {
			_, _ = r.output(context.WithoutCancel(ctx), "rm -f "+ssh.Quote(file)+"\n")
		}
	}()
	var script string
	if db, err := s.Store.DatabaseByIDAny(ctx, e.ResourceID); err == nil {
		if script, err = RestoreScript(db.Type, db.ID, e.DatabaseName, file); err != nil {
			return err
		}
	} else {
		if e.ScheduleID == nil {
			return errors.New("planification introuvable")
		}
		b, err := s.Store.BackupScheduleByIDAny(ctx, *e.ScheduleID)
		if err != nil {
			return err
		}
		script = VolumeRestoreScript(b.VolumeName, file)
	}
	if err := r.sh(ctx, "restauration de "+path.Base(e.Filename), script); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "Restauration terminée.")
	return nil
}

func (s *Service) importDump(ctx context.Context, r *run, p ImportParams) error {
	db, err := s.Store.DatabaseByIDAny(ctx, p.DatabaseID)
	if err != nil {
		return err
	}
	defer func() { _, _ = r.output(context.WithoutCancel(ctx), "rm -f "+ssh.Quote(p.File)+"\n") }()
	script, err := RestoreScript(db.Type, db.ID, p.Database, p.File)
	if err != nil {
		return err
	}
	if err := r.sh(ctx, "import de "+path.Base(p.File), script); err != nil {
		return err
	}
	fmt.Fprintln(r.out, "Import terminé.")
	return nil
}

// ImportPath : emplacement d'un fichier importé sur le serveur.
func (s *Service) ImportPath(databaseID, filename string) string {
	return s.dir() + "/" + databaseID + "/import-" + time.Now().UTC().Format("20060102-150405") + "-" + unsafeChars.ReplaceAllString(filename, "_")
}

// --- Fichiers (hors tâches) ---

// Targets fournit la cible SSH d'un serveur (servers.Service.Target).
type Targets interface {
	Target(ctx context.Context, sv *store.Server) (ssh.Target, error)
}

func (s *Service) targetFor(ctx context.Context, t Targets, serverID string) (ssh.Target, error) {
	sv, err := s.Store.ServerByIDAny(ctx, serverID)
	if err != nil {
		return ssh.Target{}, err
	}
	return t.Target(ctx, sv)
}

// Upload transfère un fichier (flux de la requête) vers le serveur, par l'entrée standard SSH.
func (s *Service) Upload(ctx context.Context, t Targets, serverID, dest string, body io.Reader) (int64, error) {
	target, err := s.targetFor(ctx, t, serverID)
	if err != nil {
		return 0, err
	}
	dir := dest[:strings.LastIndex(dest, "/")]
	var stderr bytes.Buffer
	counter := &countingReader{r: body}
	code, err := s.SSH.RunInput(ctx, target, "mkdir -p "+ssh.Quote(dir)+" && cat > "+ssh.Quote(dest), counter, io.Discard, &stderr)
	if err != nil {
		return 0, err
	}
	if code != 0 {
		return 0, fmt.Errorf("écriture sur le serveur impossible : %s", strings.TrimSpace(stderr.String()))
	}
	return counter.n, nil
}

// Stream envoie le contenu d'un fichier du serveur vers w.
func (s *Service) Stream(ctx context.Context, t Targets, serverID, file string, w io.Writer) error {
	target, err := s.targetFor(ctx, t, serverID)
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	code, err := s.SSH.Run(ctx, target, "cat "+ssh.Quote(file), w, &stderr)
	if err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("lecture impossible : %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

// Remove supprime une sauvegarde (fichier local et objet S3).
func (s *Service) Remove(ctx context.Context, t Targets, e *store.BackupExecution) error {
	if !e.LocalDeleted && e.Filename != "" {
		target, err := s.targetFor(ctx, t, e.ServerID)
		if err != nil {
			return err
		}
		if _, err := s.SSH.Output(ctx, target, "rm -f "+ssh.Quote(e.Filename)); err != nil {
			return err
		}
	}
	if e.S3Key != "" && !e.S3Deleted && e.S3StorageID != nil {
		st, err := s.Store.S3StorageByIDAny(ctx, *e.S3StorageID)
		if err == nil {
			dec, err := Decrypt(s.Box, st)
			if err != nil {
				return err
			}
			if err := dec.Remove(ctx, e.S3Key); err != nil {
				return err
			}
		}
	}
	return s.Store.MarkBackupDeleted(ctx, e.ID, true, true)
}

type countingReader struct {
	r io.Reader
	n int64
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	return n, err
}
