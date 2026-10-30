package backup

import (
	"strings"
	"testing"
	"time"

	"github.com/Murathan-Aydin/nodra/internal/store"
)

func TestDumpAndRestoreScripts(t *testing.T) {
	cases := []struct {
		typ, db string
		all     bool
		dump    string
		ext     string
	}{
		{"postgresql", "", false, `pg_dump --format=custom --no-acl --no-owner -U "$POSTGRES_USER" "$POSTGRES_DB"`, ".dump"},
		{"postgresql", "app", false, `-U "$POSTGRES_USER" '"'"'app'"'"'`, ".dump"},
		{"postgresql", "", true, `pg_dumpall -U "$POSTGRES_USER"`, ".sql.gz"},
		{"mysql", "", false, `mysqldump -u root -p"$MYSQL_ROOT_PASSWORD" --single-transaction --routines --triggers --databases "$MYSQL_DATABASE"`, ".sql.gz"},
		{"mariadb", "", true, `--all-databases`, ".sql.gz"},
		{"mongodb", "", false, `--gzip --archive --db="$MONGO_INITDB_DATABASE"`, ".archive.gz"},
		{"redis", "", false, `redis-cli -a "$REDIS_PASSWORD"`, ".rdb"},
	}
	for _, c := range cases {
		s, err := DumpScript(c.typ, "c1", c.db, c.all, "/b/x"+c.ext)
		if err != nil || !strings.Contains(s, c.dump) || Extension(c.typ, c.all) != c.ext {
			t.Errorf("%s %v : %v\n%s", c.typ, c.all, err, s)
		}
		if strings.Contains(s, "PASSWORD=") {
			t.Errorf("%s : aucun mot de passe en clair", c.typ)
		}
	}
	if _, err := DumpScript("clickhouse", "c", "", false, "/b/x"); err == nil {
		t.Fatal("clickhouse non pris en charge")
	}
	r, _ := RestoreScript("postgresql", "c1", "", "/b/x.dump")
	if !strings.Contains(r, "cat '/b/x.dump' | docker exec -i 'c1' sh -c") || !strings.Contains(r, "pg_restore --clean") {
		t.Fatalf("restauration pg : %s", r)
	}
	r, _ = RestoreScript("postgresql", "c1", "", "/b/all.sql.gz")
	if !strings.Contains(r, "gunzip -c") || !strings.Contains(r, "-d postgres") {
		t.Fatalf("restauration pg_dumpall : %s", r)
	}
	r, _ = RestoreScript("mongodb", "c1", "", "/b/x.archive.gz")
	if strings.Contains(r, "gunzip") || !strings.Contains(r, "--gzip") {
		t.Fatalf("restauration mongo : %s", r)
	}
	if _, err := RestoreScript("redis", "c", "", "/b/x.rdb"); err == nil {
		t.Fatal("restauration redis non prise en charge")
	}
	if !strings.Contains(VolumeBackupScript("v1", "/b/r/v1.tar.gz"), "-v 'v1':/data:ro") {
		t.Fatal("archive de volume en lecture seule")
	}
	if !ValidDBName("app_db") || ValidDBName("a;rm -rf /") {
		t.Fatal("validation des noms de base")
	}
}

func TestRetentionAndDue(t *testing.T) {
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	var list []*store.BackupExecution
	for i := 0; i < 5; i++ {
		list = append(list, &store.BackupExecution{ID: string(rune('a' + i)), CreatedAt: now.Add(-time.Duration(i) * 24 * time.Hour)})
	}
	if got := Expired(list, 3, 0, now); len(got) != 2 || got[0].ID != "d" {
		t.Fatalf("garder 3 : %v", got)
	}
	if got := Expired(list, 0, 2, now); len(got) != 2 || got[0].ID != "d" {
		t.Fatalf("2 jours : %d", len(got))
	}
	if got := Expired(list, 0, 0, now); len(got) != 0 {
		t.Fatal("illimité")
	}
	last := now.Add(-2 * time.Hour)
	b := &store.BackupSchedule{Frequency: "0 * * * *", LastRunAt: &last}
	if !Due(b, "Europe/Paris", now) {
		t.Fatal("horaire : dû")
	}
	recent := now.Add(-time.Minute) // 11 h 59, prochaine échéance à 12 h 00 (dû) ; puis passé à 12 h 00
	b.LastRunAt = &recent
	if !Due(b, "UTC", now) {
		t.Fatal("échéance de 12 h atteinte")
	}
	b.LastRunAt = &now
	if Due(b, "UTC", now) {
		t.Fatal("pas encore dû")
	}
	if ParseCron("pas un cron") == nil {
		t.Fatal("cron invalide")
	}
	s3 := S3{Endpoint: "https://s3.example.com", Bucket: "bk", Prefix: "nodra", AccessKey: "AK", SecretKey: "s/k+"}
	up := s3.UploadScript("/b/r/x.dump", s3.Key("t", "r", "x.dump"))
	if !strings.Contains(up, "export RCLONE_CONFIG_NODRA_SECRET_ACCESS_KEY='s/k+'") || !strings.Contains(up, "-e RCLONE_CONFIG_NODRA_SECRET_ACCESS_KEY") ||
		!strings.Contains(up, "RCLONE_CONFIG_NODRA_PROVIDER='Other'") || !strings.Contains(up, "copyto --s3-no-check-bucket '/b/r/x.dump' 'nodra:bk/nodra/t/r/x.dump'") {
		t.Fatalf("envoi S3 :\n%s", up)
	}
	run := strings.Split(strings.TrimSpace(up), "\n")
	if last := run[len(run)-1]; strings.Contains(last, "s/k+") {
		t.Fatalf("le secret ne doit pas apparaître dans la commande docker : %s", last)
	}
	aws := S3{Endpoint: "https://s3.eu-west-3.amazonaws.com", Bucket: "b"}
	if !strings.Contains(aws.UploadScript("/b/x", "k"), "PROVIDER='AWS'") {
		t.Fatal("fournisseur AWS (style virtuel)")
	}
}
