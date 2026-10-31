package api

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/rbac"
	"github.com/Murathan-Aydin/nodra/internal/store"
)

// fakeS3 : bucket "bkt" ; enregistre les suppressions d'objets.
type fakeS3 struct {
	mu      sync.Mutex
	deleted []string
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodHead && strings.Trim(r.URL.Path, "/") == "bkt":
		w.WriteHeader(200)
	case r.Method == http.MethodHead:
		w.WriteHeader(404)
	case r.Method == http.MethodDelete:
		f.mu.Lock()
		f.deleted = append(f.deleted, r.URL.Path)
		f.mu.Unlock()
		w.WriteHeader(204)
	default:
		w.WriteHeader(200)
	}
}

func TestBackups(t *testing.T) {
	f := newAppFixture(t)
	h := newFakeHost(t, f)
	f.e.backups.Dir = filepath.Join(h.state, "backups")
	f.e.runner.DatabasesDir = filepath.Join(h.state, "databases")
	f.e.api.Executors["resource.backup.restore"] = f.e.api.execRestore
	alice, base := f.alice, f.base
	bob, _ := register(t, f.e, "bob@example.com")
	invite(t, f.e, alice, bob, f.team, "bob@example.com", rbac.RoleViewer)
	s3srv := &fakeS3{}
	s3 := httptest.NewServer(s3srv)
	defer s3.Close()

	// Stockage S3 : accès vérifié à la création.
	var bad, good store.S3Storage
	expect(t, "lecteur sans storage.manage", bob.do("POST", base+"/s3-storages", map[string]any{}, nil), 403)
	expect(t, "bucket invalide", alice.do("POST", base+"/s3-storages", map[string]any{"name": "x", "endpoint": s3.URL, "bucket": "B!", "access_key": "a", "secret_key": "b"}, nil), 422)
	expect(t, "bucket absent", alice.do("POST", base+"/s3-storages", map[string]any{"name": "absent", "endpoint": s3.URL, "bucket": "nope", "access_key": "a", "secret_key": "b"}, &bad), 201)
	if bad.IsUsable || bad.LastError == "" {
		t.Fatalf("bucket absent : %+v", bad)
	}
	expect(t, "S3", alice.do("POST", base+"/s3-storages", map[string]any{"name": "S3", "endpoint": s3.URL, "bucket": "bkt", "path_prefix": "nodra",
		"access_key": "AK", "secret_key": "SK"}, &good), 201)
	if !good.IsUsable {
		t.Fatalf("bucket accessible : %+v", good)
	}

	// Base PostgreSQL démarrée sur le faux hôte.
	var db map[string]any
	expect(t, "base", alice.do("POST", base+"/databases", map[string]any{"environment_id": f.envID, "destination_id": f.destID, "type": "postgresql", "name": "pg"}, &db), 201)
	dbID := db["id"].(string)

	// Planification : validation, puis exécution immédiate vers S3 sans copie locale.
	sched := map[string]any{"kind": "database", "resource_id": dbID, "frequency": "0 3 * * *", "s3_storage_id": good.ID, "local_backup": false, "keep_s3": 2}
	expect(t, "lecteur sans resource.backup.manage", bob.do("POST", base+"/backups", sched, nil), 403)
	expect(t, "cron invalide", alice.do("POST", base+"/backups", map[string]any{"kind": "database", "resource_id": dbID, "frequency": "jamais"}, nil), 422)
	expect(t, "sans S3 ni local", alice.do("POST", base+"/backups", map[string]any{"kind": "database", "resource_id": dbID, "local_backup": false}, nil), 422)
	expect(t, "nom de base invalide", alice.do("POST", base+"/backups", map[string]any{"kind": "database", "resource_id": dbID, "databases": "a;b"}, nil), 422)
	var b store.BackupSchedule
	expect(t, "planification", alice.do("POST", base+"/backups", sched, &b), 201)
	var upd store.BackupSchedule
	expect(t, "modification", alice.do("PATCH", base+"/backups/"+b.ID, map[string]any{"frequency": "0 4 * * *", "keep_local": 3}, &upd), 200)
	if upd.Frequency != "0 4 * * *" || upd.KeepLocal != 3 || upd.S3StorageID == nil {
		t.Fatalf("modification : %+v", upd)
	}
	run := func() store.ServerTask {
		t.Helper()
		var tk store.ServerTask
		expect(t, "sauvegarde", alice.do("POST", base+"/backups/"+b.ID+"/run", nil, &tk), 202)
		return waitTask(t, alice, f.team, f.serverID, tk.ID)
	}
	tk := run()
	if tk.Status != "success" || !strings.Contains(tk.Output, "envoi vers S3") {
		t.Fatalf("sauvegarde : %s %s\n%s", tk.Status, tk.Error, tk.Output)
	}
	calls := h.calls()
	if !strings.Contains(calls, "docker exec "+dbID+" sh -c pg_dump --format=custom") || strings.Contains(calls, "AK:SK") || strings.Contains(calls, "SK@") {
		t.Fatalf("dump dans le conteneur, secret S3 absent des arguments :\n%s", calls)
	}
	s3log, _ := os.ReadFile(filepath.Join(h.state, "s3"))
	if !strings.Contains(string(s3log), "nodra:bkt/nodra/"+f.team+"/"+dbID+"/default-") {
		t.Fatalf("clé S3 : %s", s3log)
	}
	var execs []store.BackupExecution
	expect(t, "exécutions", bob.do("GET", base+"/backup-executions?resource_id="+dbID, nil, &execs), 200)
	if len(execs) != 1 || execs[0].Status != "success" || !execs[0].LocalDeleted || execs[0].S3Key == "" || execs[0].Size == 0 {
		t.Fatalf("exécution : %+v", execs)
	}

	// Rétention S3 : 2 sauvegardes conservées.
	time.Sleep(1100 * time.Millisecond)
	run()
	time.Sleep(1100 * time.Millisecond)
	run()
	s3srv.mu.Lock()
	deleted := append([]string(nil), s3srv.deleted...)
	s3srv.mu.Unlock()
	if len(deleted) != 1 || !strings.Contains(deleted[0], execs[0].S3Key) {
		t.Fatalf("rétention S3 : %v", deleted)
	}

	// Téléchargement : lien S3 temporaire.
	expect(t, "exécutions", alice.do("GET", base+"/backup-executions?resource_id="+dbID, nil, &execs), 200)
	var dl struct{ URL string }
	expect(t, "lecteur sans téléchargement", bob.do("GET", base+"/backup-executions/"+execs[0].ID+"/download", nil, nil), 403)
	expect(t, "téléchargement", alice.do("GET", base+"/backup-executions/"+execs[0].ID+"/download", nil, &dl), 200)
	if !strings.Contains(dl.URL, "X-Amz-Signature") {
		t.Fatalf("lien présigné : %s", dl.URL)
	}

	// Restauration soumise à approbation : fichier récupéré depuis S3 puis pg_restore.
	dave, _ := register(t, f.e, "dave@example.com")
	invite(t, f.e, alice, dave, f.team, "dave@example.com", rbac.RoleAdmin)
	expect(t, "règle", alice.do("POST", base+"/approval-rules", map[string]any{"name": "Restauration", "actions": []string{"resource.backup.restore"}}, nil), 201)
	var pending struct {
		ApprovalRequest store.ApprovalRequest `json:"approval_request"`
	}
	expect(t, "restauration soumise", alice.do("POST", base+"/backup-executions/"+execs[0].ID+"/restore", nil, &pending), 202)
	var appr store.ApprovalRequest
	expect(t, "approbation", dave.do("POST", base+"/approvals/"+pending.ApprovalRequest.ID+"/approve", nil, &appr), 200)
	if appr.Status != "executed" {
		t.Fatalf("approbation : %s %s", appr.Status, appr.Error)
	}
	var tasks []store.ServerTask
	expect(t, "tâches serveur", alice.do("GET", base+"/servers/"+f.serverID+"/tasks", nil, &tasks), 200)
	restore := waitTask(t, alice, f.team, f.serverID, tasks[0].ID)
	if restore.Kind != "backup_restore" || restore.Status != "success" || !strings.Contains(restore.Output, "téléchargement depuis S3") {
		t.Fatalf("restauration : %s %s %s\n%s", restore.Kind, restore.Status, restore.Error, restore.Output)
	}
	if !strings.Contains(h.calls(), "pg_restore --clean --if-exists") {
		t.Fatal("pg_restore attendu")
	}
	expect(t, "suppression règle", alice.do("DELETE", base+"/approval-rules/"+*appr.RuleID, nil, nil), 204)

	// Sauvegarde locale téléchargée depuis le serveur, puis supprimée.
	var local store.BackupSchedule
	expect(t, "planification locale", alice.do("POST", base+"/backups", map[string]any{"kind": "database", "resource_id": dbID, "dump_all": true}, &local), 201)
	var ltk store.ServerTask
	expect(t, "sauvegarde locale", alice.do("POST", base+"/backups/"+local.ID+"/run", nil, &ltk), 202)
	if ltk = waitTask(t, alice, f.team, f.serverID, ltk.ID); ltk.Status != "success" {
		t.Fatalf("sauvegarde locale : %s", ltk.Error)
	}
	expect(t, "exécutions", alice.do("GET", base+"/backup-executions?resource_id="+dbID, nil, &execs), 200)
	if !strings.HasSuffix(execs[0].Filename, ".sql.gz") || execs[0].LocalDeleted {
		t.Fatalf("dump complet local : %+v", execs[0])
	}
	req, _ := http.NewRequest("GET", alice.base+base+"/backup-executions/"+execs[0].ID+"/download", nil)
	resp, err := alice.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(resp.Header.Get("Content-Disposition"), ".sql.gz") || len(body) == 0 {
		t.Fatalf("téléchargement local : %d %q", resp.StatusCode, body)
	}
	expect(t, "suppression sauvegarde", alice.do("DELETE", base+"/backup-executions/"+execs[0].ID, nil, nil), 204)
	if _, err := os.Stat(execs[0].Filename); !os.IsNotExist(err) {
		t.Fatal("fichier local supprimé")
	}

	// Import d'un dump (corps brut) envoyé vers le serveur puis restauré.
	expect(t, "extension refusée", alice.do("POST", base+"/databases/"+dbID+"/import?filename=x.exe", map[string]any{}, nil), 422)
	req, _ = http.NewRequest("POST", alice.base+base+"/databases/"+dbID+"/import?filename=prod.dump", bytes.NewReader([]byte("PGDMP contenu")))
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Requested-With", "nodra-test")
	resp, err = alice.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var itk store.ServerTask
	_ = jsonDecode(resp.Body, &itk)
	resp.Body.Close()
	if resp.StatusCode != 202 {
		t.Fatalf("import : %d", resp.StatusCode)
	}
	if itk = waitTask(t, alice, f.team, f.serverID, itk.ID); itk.Status != "success" || !strings.Contains(itk.Output, "Import terminé") {
		t.Fatalf("import : %s %s\n%s", itk.Status, itk.Error, itk.Output)
	}
	matches, _ := filepath.Glob(filepath.Join(h.state, "backups", dbID, "import-*"))
	if len(matches) != 0 {
		t.Fatalf("le fichier importé est supprimé après restauration : %v", matches)
	}

	// Sauvegarde d'un volume de l'application.
	var app map[string]any
	expect(t, "application", alice.do("POST", base+"/applications", f.app(nil), &app), 201)
	appID := app["id"].(string)
	expect(t, "volume", alice.do("POST", base+"/applications/"+appID+"/storages", map[string]any{"kind": "volume", "name": "uploads", "mount_path": "/data"}, nil), 201)
	expect(t, "volume étranger", alice.do("POST", base+"/backups", map[string]any{"kind": "volume", "resource_id": appID, "volume_name": "autre"}, nil), 422)
	var vb store.BackupSchedule
	expect(t, "planification volume", alice.do("POST", base+"/backups", map[string]any{"kind": "volume", "resource_id": appID, "volume_name": "uploads"}, &vb), 201)
	var vtk store.ServerTask
	expect(t, "sauvegarde volume", alice.do("POST", base+"/backups/"+vb.ID+"/run", nil, &vtk), 202)
	if vtk = waitTask(t, alice, f.team, f.serverID, vtk.ID); vtk.Status != "success" {
		t.Fatalf("volume : %s %s\n%s", vtk.Status, vtk.Error, vtk.Output)
	}

	// Planificateur : une planification due est enfilée une seule fois.
	if err := f.e.jobs.ScheduleBackups(t.Context(), time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := f.e.jobs.ScheduleBackups(t.Context(), time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, id := range []string{b.ID, local.ID, vb.ID} {
		list, _ := f.e.st.TasksWithParam(t.Context(), "schedule_id", id, 100)
		for _, tk := range list {
			if tk.CreatedBy == nil {
				n++
			}
		}
	}
	if n != 3 {
		t.Fatalf("3 sauvegardes planifiées attendues, %d", n)
	}
}
