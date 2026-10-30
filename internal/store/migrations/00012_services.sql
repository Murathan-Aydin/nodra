-- +goose Up
CREATE TABLE services (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id            uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    environment_id     uuid NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    destination_id     uuid NOT NULL REFERENCES destinations(id) ON DELETE RESTRICT,
    name               text NOT NULL,
    description        text NOT NULL DEFAULT '',
    template           text NOT NULL DEFAULT '', -- modèle d'origine (vide = compose personnalisé)
    docker_compose_raw text NOT NULL,
    status             text NOT NULL DEFAULT 'stopped',
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX services_team_idx ON services (team_id);
CREATE INDEX services_env_idx ON services (environment_id);

-- Sous-services du compose : domaines, port routé, variable d'URL associée.
CREATE TABLE service_apps (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    service_id          uuid NOT NULL REFERENCES services(id) ON DELETE CASCADE,
    name                text NOT NULL,
    image               text NOT NULL DEFAULT '',
    domains             text[] NOT NULL DEFAULT '{}',
    port                text NOT NULL DEFAULT '',
    url_var             text NOT NULL DEFAULT '', -- identifiant X de SERVICE_URL_X / SERVICE_FQDN_X
    exclude_from_status boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE (service_id, name)
);

CREATE OR REPLACE VIEW resources AS
    SELECT id, team_id, environment_id, destination_id, name, 'application'::text AS type FROM applications
    UNION ALL
    SELECT id, team_id, environment_id, destination_id, name, 'database'::text AS type FROM databases
    UNION ALL
    SELECT id, team_id, environment_id, destination_id, name, 'service'::text AS type FROM services;

CREATE INDEX server_tasks_service_idx ON server_tasks ((params->>'service_id'), created_at DESC)
    WHERE params ? 'service_id';

-- +goose Down
DROP INDEX server_tasks_service_idx;
CREATE OR REPLACE VIEW resources AS
    SELECT id, team_id, environment_id, destination_id, name, 'application'::text AS type FROM applications
    UNION ALL
    SELECT id, team_id, environment_id, destination_id, name, 'database'::text AS type FROM databases;
DROP TABLE service_apps;
DROP TABLE services;
