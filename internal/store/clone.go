package store

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// column : colonne copiable d'une table (type SQL exact pour typer les valeurs remplacées).
type column struct{ name, typ string }

// copyColumns : colonnes d'une table hors id, created_at et updated_at (et colonnes générées).
func copyColumns(ctx context.Context, tx pgx.Tx, table string) ([]column, error) {
	rows, err := tx.Query(ctx, `SELECT a.attname, format_type(a.atttypid, a.atttypmod) FROM pg_attribute a
		WHERE a.attrelid = $1::regclass AND a.attnum > 0 AND NOT a.attisdropped AND a.attgenerated = ''
		  AND a.attname NOT IN ('id', 'created_at', 'updated_at') ORDER BY a.attnum`, table)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (column, error) {
		var c column
		err := row.Scan(&c.name, &c.typ)
		return c, err
	})
}

// cloneRows copie les lignes de table où whereCol = src, avec les colonnes de overrides remplacées ;
// renvoie les identifiants créés si withID (table avec colonne id).
func cloneRows(ctx context.Context, tx pgx.Tx, table, whereCol, src string, overrides map[string]any, withID bool) ([]string, error) {
	cols, err := copyColumns(ctx, tx, table)
	if err != nil {
		return nil, err
	}
	args := []any{src}
	names := make([]string, len(cols))
	sel := make([]string, len(cols))
	for i, c := range cols {
		names[i] = pgx.Identifier{c.name}.Sanitize()
		if v, ok := overrides[c.name]; ok {
			args = append(args, v)
			sel[i] = fmt.Sprintf("$%d::%s", len(args), c.typ)
		} else {
			sel[i] = names[i]
		}
	}
	t := pgx.Identifier{table}.Sanitize()
	sql := fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s WHERE %s = $1`, t, strings.Join(names, ", "), strings.Join(sel, ", "),
		t, pgx.Identifier{whereCol}.Sanitize())
	if !withID {
		_, err := tx.Exec(ctx, sql, args...)
		return nil, err
	}
	rows, err := tx.Query(ctx, sql+" RETURNING id::text", args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// cloneVolumeName : nom de volume Docker d'une copie (les volumes sont globaux sur le serveur) :
// l'identifiant de la ressource d'origine est remplacé, sinon le nom reçoit un suffixe.
func cloneVolumeName(name, src, dst string) string {
	compact := func(id string) string { return strings.ReplaceAll(id, "-", "")[:12] }
	out := strings.ReplaceAll(name, src, dst)
	out = strings.ReplaceAll(out, compact(src), compact(dst))
	if out == name {
		out = name + "-" + compact(dst)[:8]
	}
	return out
}

// CloneResource copie une ressource (table : applications, databases ou services) avec ses variables,
// stockages (volumes renommés : les données ne sont pas copiées), tags, tâches planifiées (désactivées)
// et, pour un service, ses sous-services. overrides remplace des colonnes de la ressource ;
// appOverrides celles des sous-services. Renvoie l'identifiant de la copie.
func (s *Store) CloneResource(ctx context.Context, table, src string, overrides, appOverrides map[string]any) (string, error) {
	var id string
	err := pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		ids, err := cloneRows(ctx, tx, table, "id", src, overrides, true)
		if err != nil {
			return err
		}
		if len(ids) != 1 {
			return ErrNotFound
		}
		id = ids[0]
		for _, child := range []struct {
			table     string
			overrides map[string]any
			withID    bool
		}{
			{"resource_env_vars", map[string]any{"resource_id": id}, true},
			{"resource_storages", map[string]any{"resource_id": id}, true},
			{"resource_tags", map[string]any{"resource_id": id}, false},
			{"scheduled_tasks", map[string]any{"resource_id": id, "enabled": false, "last_run_at": nil}, true},
		} {
			if _, err := cloneRows(ctx, tx, child.table, "resource_id", src, child.overrides, child.withID); err != nil {
				return fmt.Errorf("copie de %s : %w", child.table, err)
			}
		}
		if table == "services" {
			o := map[string]any{"service_id": id}
			for k, v := range appOverrides {
				o[k] = v
			}
			if _, err := cloneRows(ctx, tx, "service_apps", "service_id", src, o, true); err != nil {
				return fmt.Errorf("copie des sous-services : %w", err)
			}
		}
		rows, err := tx.Query(ctx, `SELECT id::text, name FROM resource_storages WHERE resource_id = $1 AND kind = 'volume'`, id)
		if err != nil {
			return err
		}
		type vol struct{ id, name string }
		vols, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (vol, error) {
			var v vol
			err := row.Scan(&v.id, &v.name)
			return v, err
		})
		if err != nil {
			return err
		}
		for _, v := range vols {
			if _, err := tx.Exec(ctx, `UPDATE resource_storages SET name = $2 WHERE id = $1`, v.id, cloneVolumeName(v.name, src, id)); err != nil {
				return err
			}
		}
		return nil
	})
	if isForeignKeyViolation(err) { // environnement ou destination disparus entre-temps
		return "", ErrNotFound
	}
	return id, err
}
