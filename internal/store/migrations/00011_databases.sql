-- +goose Up
-- Bases de données autonomes (une table pour tous les types ; options propres à un type dans settings).
CREATE TABLE databases (
    id                        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id                   uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    environment_id            uuid NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    destination_id            uuid NOT NULL REFERENCES destinations(id) ON DELETE RESTRICT,
    type                      text NOT NULL CHECK (type IN ('postgresql', 'mysql', 'mariadb', 'mongodb', 'redis', 'keydb', 'dragonfly', 'clickhouse')),
    name                      text NOT NULL,
    description               text NOT NULL DEFAULT '',
    image                     text NOT NULL,
    status                    text NOT NULL DEFAULT 'stopped',
    username                  text NOT NULL DEFAULT '',
    password_enc              text NOT NULL,
    root_password_enc         text NOT NULL DEFAULT '', -- MySQL / MariaDB
    database_name             text NOT NULL DEFAULT '',
    custom_config             text NOT NULL DEFAULT '', -- postgresql.conf, my.cnf, mongod.conf, redis.conf…
    init_scripts              jsonb NOT NULL DEFAULT '[]', -- [{filename, content}]
    is_public                 boolean NOT NULL DEFAULT false,
    public_port               int CHECK (public_port BETWEEN 1 AND 65535),
    ports_mappings            text NOT NULL DEFAULT '',
    custom_docker_run_options text NOT NULL DEFAULT '',
    limits                    jsonb NOT NULL DEFAULT '{}',
    settings                  jsonb NOT NULL DEFAULT '{}',
    started_at                timestamptz,
    created_at                timestamptz NOT NULL DEFAULT now(),
    updated_at                timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX databases_team_idx ON databases (team_id);
CREATE INDEX databases_env_idx ON databases (environment_id);

CREATE OR REPLACE VIEW resources AS
    SELECT id, team_id, environment_id, destination_id, name, 'application'::text AS type FROM applications
    UNION ALL
    SELECT id, team_id, environment_id, destination_id, name, 'database'::text AS type FROM databases;

-- Actions sur une base (démarrage, arrêt…) : tâches serveur portant params.database_id.
CREATE INDEX server_tasks_database_idx ON server_tasks ((params->>'database_id'), created_at DESC)
    WHERE params ? 'database_id';

-- +goose Down
DROP INDEX server_tasks_database_idx;
CREATE OR REPLACE VIEW resources AS
    SELECT id, team_id, environment_id, destination_id, name, 'application'::text AS type FROM applications;
DROP TABLE databases;
