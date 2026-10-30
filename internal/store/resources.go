package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// --- Variables d'environnement des ressources ---

type EnvVar struct {
	ID           string    `json:"id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Key          string    `json:"key"`
	ValueEnc     string    `json:"-"`
	Value        string    `json:"value"` // vide pour un secret
	IsBuildtime  bool      `json:"is_buildtime"`
	IsRuntime    bool      `json:"is_runtime"`
	IsPreview    bool      `json:"is_preview"`
	IsLiteral    bool      `json:"is_literal"`
	IsMultiline  bool      `json:"is_multiline"`
	IsSecret     bool      `json:"is_secret"`
	Comment      string    `json:"comment"`
	UpdatedAt    time.Time `json:"updated_at"`
}

const envVarCols = `id::text, resource_type, resource_id::text, key, value_enc, is_buildtime, is_runtime, is_preview, is_literal,
	is_multiline, is_secret, comment, updated_at`

func scanEnvVar(row pgx.Row) (*EnvVar, error) {
	v := &EnvVar{}
	err := row.Scan(&v.ID, &v.ResourceType, &v.ResourceID, &v.Key, &v.ValueEnc, &v.IsBuildtime, &v.IsRuntime, &v.IsPreview,
		&v.IsLiteral, &v.IsMultiline, &v.IsSecret, &v.Comment, &v.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return v, err
}

func (s *Store) ListEnvVars(ctx context.Context, resourceID string) ([]*EnvVar, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+envVarCols+` FROM resource_env_vars WHERE resource_id = $1 ORDER BY is_preview, key`, resourceID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanEnvVar)
}

func (s *Store) EnvVarByID(ctx context.Context, resourceID, id string) (*EnvVar, error) {
	return scanEnvVar(s.Pool.QueryRow(ctx, `SELECT `+envVarCols+` FROM resource_env_vars WHERE resource_id = $1 AND id = $2`, resourceID, id))
}

// UpsertEnvVar crée ou remplace la variable (clé + préversion).
func (s *Store) UpsertEnvVar(ctx context.Context, v *EnvVar) (*EnvVar, error) {
	return scanEnvVar(s.Pool.QueryRow(ctx, `INSERT INTO resource_env_vars (resource_type, resource_id, key, value_enc, is_buildtime,
		is_runtime, is_preview, is_literal, is_multiline, is_secret, comment) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (resource_type, resource_id, key, is_preview) DO UPDATE SET value_enc = EXCLUDED.value_enc,
		is_buildtime = EXCLUDED.is_buildtime, is_runtime = EXCLUDED.is_runtime, is_literal = EXCLUDED.is_literal,
		is_multiline = EXCLUDED.is_multiline, is_secret = resource_env_vars.is_secret OR EXCLUDED.is_secret,
		comment = EXCLUDED.comment, updated_at = now() RETURNING `+envVarCols,
		v.ResourceType, v.ResourceID, v.Key, v.ValueEnc, v.IsBuildtime, v.IsRuntime, v.IsPreview, v.IsLiteral, v.IsMultiline,
		v.IsSecret, v.Comment))
}

func (s *Store) UpdateEnvVar(ctx context.Context, v *EnvVar) error {
	_, err := s.Pool.Exec(ctx, `UPDATE resource_env_vars SET value_enc = $3, is_buildtime = $4, is_runtime = $5, is_literal = $6,
		is_multiline = $7, is_secret = $8, comment = $9, updated_at = now() WHERE resource_id = $1 AND id = $2`,
		v.ResourceID, v.ID, v.ValueEnc, v.IsBuildtime, v.IsRuntime, v.IsLiteral, v.IsMultiline, v.IsSecret, v.Comment)
	return err
}

func (s *Store) DeleteEnvVar(ctx context.Context, resourceID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM resource_env_vars WHERE resource_id = $1 AND id = $2`, resourceID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// --- Stockages ---

type Storage struct {
	ID           string    `json:"id"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	Kind         string    `json:"kind"`
	Name         string    `json:"name"`
	HostPath     string    `json:"host_path"`
	MountPath    string    `json:"mount_path"`
	Content      string    `json:"content"`
	CreatedAt    time.Time `json:"created_at"`
}

const storageCols = `id::text, resource_type, resource_id::text, kind, name, host_path, mount_path, content, created_at`

func scanStorage(row pgx.Row) (*Storage, error) {
	st := &Storage{}
	err := row.Scan(&st.ID, &st.ResourceType, &st.ResourceID, &st.Kind, &st.Name, &st.HostPath, &st.MountPath, &st.Content, &st.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return st, err
}

func (s *Store) ListStorages(ctx context.Context, resourceID string) ([]*Storage, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+storageCols+` FROM resource_storages WHERE resource_id = $1 ORDER BY mount_path`, resourceID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanStorage)
}

func (s *Store) CreateStorage(ctx context.Context, st *Storage) (*Storage, error) {
	r, err := scanStorage(s.Pool.QueryRow(ctx, `INSERT INTO resource_storages (resource_type, resource_id, kind, name, host_path,
		mount_path, content) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING `+storageCols,
		st.ResourceType, st.ResourceID, st.Kind, st.Name, st.HostPath, st.MountPath, st.Content))
	if isUniqueViolation(err) {
		return nil, ErrConflict
	}
	return r, err
}

func (s *Store) UpdateStorageContent(ctx context.Context, resourceID, id, content string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE resource_storages SET content = $3 WHERE resource_id = $1 AND id = $2 AND kind = 'file'`,
		resourceID, id, content)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) DeleteStorage(ctx context.Context, resourceID, id string) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM resource_storages WHERE resource_id = $1 AND id = $2`, resourceID, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

// --- Tags des ressources ---

func (s *Store) ResourceTags(ctx context.Context, resourceID string) ([]*Tag, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t.id::text, t.team_id::text, t.name, t.color, t.created_at
		FROM tags t JOIN resource_tags rt ON rt.tag_id = t.id WHERE rt.resource_id = $1 ORDER BY lower(t.name)`, resourceID)
	if err != nil {
		return nil, err
	}
	return collect(rows, scanTag)
}

// SetResourceTags remplace les tags d'une ressource (tags de l'équipe uniquement).
func (s *Store) SetResourceTags(ctx context.Context, teamID, resourceType, resourceID string, tagIDs []string) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM resource_tags WHERE resource_id = $1`, resourceID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO resource_tags (tag_id, resource_type, resource_id)
			SELECT id, $3, $4 FROM tags WHERE team_id = $1 AND id = ANY($2::uuid[])`, teamID, tagIDs, resourceType, resourceID)
		return err
	})
}
