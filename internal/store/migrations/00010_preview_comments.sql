-- +goose Up
-- Commentaire publié sur la pull request (mis à jour à chaque déploiement de la preview).
ALTER TABLE application_previews ADD COLUMN comment_id bigint NOT NULL DEFAULT 0;

-- +goose Down
ALTER TABLE application_previews DROP COLUMN comment_id;
