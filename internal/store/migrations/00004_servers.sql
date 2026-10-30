-- +goose Up
CREATE TABLE private_keys (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name            text NOT NULL,
    description     text NOT NULL DEFAULT '',
    private_key_enc text NOT NULL, -- chiffrée (AES-GCM)
    public_key      text NOT NULL,
    fingerprint     text NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX private_keys_team_name ON private_keys (team_id, lower(name));

CREATE TABLE servers (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id          uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name             text NOT NULL,
    description      text NOT NULL DEFAULT '',
    ip               text NOT NULL,
    port             int NOT NULL DEFAULT 22 CHECK (port BETWEEN 1 AND 65535),
    ssh_user         text NOT NULL DEFAULT 'root',
    private_key_id   uuid NOT NULL REFERENCES private_keys(id) ON DELETE RESTRICT,
    host_key         text NOT NULL DEFAULT '',
    is_reachable     boolean NOT NULL DEFAULT false,
    is_usable        boolean NOT NULL DEFAULT false,
    os_id            text NOT NULL DEFAULT '',
    os_version       text NOT NULL DEFAULT '',
    docker_version   text NOT NULL DEFAULT '',
    compose_version  text NOT NULL DEFAULT '',
    validation_error text NOT NULL DEFAULT '',
    validated_at     timestamptz,
    last_seen_at     timestamptz,
    last_cleanup_at  timestamptz,
    settings         jsonb NOT NULL DEFAULT '{}',
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX servers_team_idx ON servers (team_id);
CREATE UNIQUE INDEX servers_team_name ON servers (team_id, lower(name));

-- Destinations : réseaux Docker d'un serveur dans lesquels les ressources sont déployées.
CREATE TABLE destinations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id  uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name       text NOT NULL,
    network    text NOT NULL,
    is_default boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (server_id, network)
);

-- Tâches exécutées sur un serveur (validation, installation, nettoyage, commandes…) avec leur sortie.
CREATE TABLE server_tasks (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    server_id   uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    kind        text NOT NULL,
    params      jsonb NOT NULL DEFAULT '{}',
    status      text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'success', 'failed', 'cancelled')),
    output      text NOT NULL DEFAULT '',
    error       text NOT NULL DEFAULT '',
    created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    started_at  timestamptz,
    finished_at timestamptz
);
CREATE INDEX server_tasks_server_idx ON server_tasks (server_id, created_at DESC);

CREATE TABLE server_metrics (
    server_id   uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    ts          timestamptz NOT NULL,
    cpu_percent real NOT NULL,
    mem_used    bigint NOT NULL,
    mem_total   bigint NOT NULL,
    disk_used   bigint NOT NULL,
    disk_total  bigint NOT NULL,
    load1       real NOT NULL,
    PRIMARY KEY (server_id, ts)
);

-- +goose Down
DROP TABLE server_metrics;
DROP TABLE server_tasks;
DROP TABLE destinations;
DROP TABLE servers;
DROP TABLE private_keys;
