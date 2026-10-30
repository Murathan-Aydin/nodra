-- +goose Up
-- Commandes planifiées exécutées dans un conteneur d'une ressource (historique : server_tasks, params.scheduled_task_id).
CREATE TABLE scheduled_tasks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    resource_id uuid NOT NULL,
    name        text NOT NULL,
    command     text NOT NULL,
    frequency   text NOT NULL,
    container   text NOT NULL DEFAULT '', -- service compose ciblé (services et applications compose)
    timeout     int NOT NULL DEFAULT 3600 CHECK (timeout BETWEEN 10 AND 86400),
    enabled     boolean NOT NULL DEFAULT true,
    last_run_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX scheduled_tasks_resource_idx ON scheduled_tasks (resource_id);
CREATE INDEX server_tasks_scheduled_idx ON server_tasks ((params->>'scheduled_task_id'), created_at DESC)
    WHERE params ? 'scheduled_task_id';

-- +goose Down
DROP INDEX server_tasks_scheduled_idx;
DROP TABLE scheduled_tasks;
