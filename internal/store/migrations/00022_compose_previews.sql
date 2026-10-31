-- +goose Up
-- Domaines par service compose d'une prévisualisation (applications Docker Compose).
ALTER TABLE application_previews ADD COLUMN compose_domains jsonb NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE application_previews DROP COLUMN compose_domains;
