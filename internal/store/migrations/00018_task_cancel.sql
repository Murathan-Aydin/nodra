-- +goose Up
-- Annulation demandée pour une tâche serveur en cours (relue par le processus qui l'exécute).
ALTER TABLE server_tasks ADD COLUMN cancel_requested boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE server_tasks DROP COLUMN cancel_requested;
