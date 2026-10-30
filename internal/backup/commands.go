// Package backup sauvegarde les bases (dumps par type) et les volumes Docker, les envoie vers S3,
// applique la rétention et restaure.
package backup

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Murathan-Aydin/nodra/internal/ssh"
)

// Dir : dossier des sauvegardes sur les serveurs.
const Dir = "/data/nodra/backups"

var dbNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.$-]{0,62}$`)

// ValidDBName : nom de base acceptable dans une commande (ex. liste « databases »).
func ValidDBName(s string) bool { return dbNameRe.MatchString(s) }

// Supported indique si un type de base peut être sauvegardé, et restauré.
func Supported(typ string) (backup, restore bool) {
	switch typ {
	case "postgresql", "mysql", "mariadb", "mongodb":
		return true, true
	case "redis", "keydb":
		return true, false // restauration manuelle (remplacement du fichier RDB à l'arrêt)
	}
	return false, false
}

// Extension du fichier produit par DumpScript.
func Extension(typ string, all bool) string {
	switch typ {
	case "postgresql":
		if all {
			return ".sql.gz"
		}
		return ".dump"
	case "mysql", "mariadb":
		return ".sql.gz"
	case "mongodb":
		return ".archive.gz"
	case "redis", "keydb":
		return ".rdb"
	}
	return ""
}

// inContainer exécute une commande dans le conteneur ; les identifiants sont lus dans son environnement
// (jamais dans les arguments visibles sur l'hôte).
func inContainer(container, cmd string, stdin bool) string {
	i := ""
	if stdin {
		i = "-i "
	}
	return "docker exec " + i + ssh.Quote(container) + " sh -c " + ssh.Quote(cmd)
}

// DumpScript produit le script qui écrit la sauvegarde dans file. db = nom de la base ("" = base par défaut).
func DumpScript(typ, container, db string, all bool, file string) (string, error) {
	q := ssh.Quote(file)
	pre := fmt.Sprintf("mkdir -p %s\n", ssh.Quote(file[:strings.LastIndex(file, "/")]))
	dbArg := func(def string) string {
		if db == "" {
			return def
		}
		return ssh.Quote(db)
	}
	switch typ {
	case "postgresql":
		if all {
			return pre + inContainer(container, `pg_dumpall -U "$POSTGRES_USER"`, false) + " | gzip > " + q + "\n", nil
		}
		return pre + inContainer(container, `pg_dump --format=custom --no-acl --no-owner -U "$POSTGRES_USER" `+dbArg(`"$POSTGRES_DB"`), false) +
			" > " + q + "\n", nil
	case "mysql", "mariadb":
		bin, pass := "mysqldump", `"$MYSQL_ROOT_PASSWORD"`
		if typ == "mariadb" {
			bin, pass = "$(command -v mariadb-dump || command -v mysqldump)", `"$MARIADB_ROOT_PASSWORD"`
		}
		target := "--all-databases"
		if !all {
			def := `"$MYSQL_DATABASE"`
			if typ == "mariadb" {
				def = `"$MARIADB_DATABASE"`
			}
			target = "--databases " + dbArg(def)
		}
		return pre + inContainer(container, bin+` -u root -p`+pass+` --single-transaction --routines --triggers `+target, false) +
			" | gzip > " + q + "\n", nil
	case "mongodb":
		cmd := `mongodump --authenticationDatabase=admin -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD" --gzip --archive`
		if !all {
			cmd += " --db=" + dbArg(`"$MONGO_INITDB_DATABASE"`)
		}
		return pre + inContainer(container, cmd, false) + " > " + q + "\n", nil
	case "redis", "keydb":
		cli := "redis-cli"
		if typ == "keydb" {
			cli = "keydb-cli"
		}
		return pre + inContainer(container, cli+` -a "$REDIS_PASSWORD" --no-auth-warning --rdb /tmp/nodra-backup.rdb >/dev/null && cat /tmp/nodra-backup.rdb && rm -f /tmp/nodra-backup.rdb`, false) +
			" > " + q + "\n", nil
	}
	return "", fmt.Errorf("sauvegarde non prise en charge pour %s", typ)
}

// RestoreScript restaure file (format déduit de l'extension) dans la base db ("" = base par défaut).
func RestoreScript(typ, container, db, file string) (string, error) {
	q := ssh.Quote(file)
	dbArg := func(def string) string {
		if db == "" {
			return def
		}
		return ssh.Quote(db)
	}
	gz := strings.HasSuffix(file, ".gz")
	feed := "cat " + q + " | "
	if gz && typ != "mongodb" {
		feed = "gunzip -c " + q + " | "
	}
	switch typ {
	case "postgresql":
		if strings.HasSuffix(file, ".dump") {
			return feed + inContainer(container, `pg_restore --clean --if-exists --no-acl --no-owner -U "$POSTGRES_USER" -d `+dbArg(`"$POSTGRES_DB"`), true) + "\n", nil
		}
		target := dbArg(`"$POSTGRES_DB"`)
		if gz && db == "" {
			target = "postgres" // dump complet (pg_dumpall) : rejoué depuis la base postgres
		}
		return feed + inContainer(container, `psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d `+target, true) + "\n", nil
	case "mysql", "mariadb":
		bin, pass := "mysql", `"$MYSQL_ROOT_PASSWORD"`
		if typ == "mariadb" {
			bin, pass = "$(command -v mariadb || command -v mysql)", `"$MARIADB_ROOT_PASSWORD"`
		}
		target := ""
		if db != "" {
			target = " " + ssh.Quote(db)
		}
		return feed + inContainer(container, bin+` -u root -p`+pass+target, true) + "\n", nil
	case "mongodb":
		cmd := `mongorestore --authenticationDatabase=admin -u "$MONGO_INITDB_ROOT_USERNAME" -p "$MONGO_INITDB_ROOT_PASSWORD" --drop --archive`
		if gz {
			cmd += " --gzip"
		}
		return feed + inContainer(container, cmd, true) + "\n", nil
	}
	return "", fmt.Errorf("restauration non prise en charge pour %s", typ)
}

// VolumeBackupScript archive un volume Docker (lecture seule) dans file.
func VolumeBackupScript(volume, file string) string {
	dir, name := file[:strings.LastIndex(file, "/")], file[strings.LastIndex(file, "/")+1:]
	return fmt.Sprintf("mkdir -p %s\ndocker volume inspect %s >/dev/null\ndocker run --rm -v %s:/data:ro -v %s:/backup alpine:3 tar czf %s -C /data .\n",
		ssh.Quote(dir), ssh.Quote(volume), ssh.Quote(volume), ssh.Quote(dir), ssh.Quote("/backup/"+name))
}

// VolumeRestoreScript remplace le contenu du volume par l'archive file.
func VolumeRestoreScript(volume, file string) string {
	dir, name := file[:strings.LastIndex(file, "/")], file[strings.LastIndex(file, "/")+1:]
	return fmt.Sprintf("docker volume inspect %s >/dev/null\ndocker run --rm -v %s:/data -v %s:/backup alpine:3 sh -c %s\n",
		ssh.Quote(volume), ssh.Quote(volume), ssh.Quote(dir), ssh.Quote("find /data -mindepth 1 -delete && tar xzf /backup/"+name+" -C /data"))
}
