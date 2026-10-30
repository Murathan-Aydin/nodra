-- +goose Up
CREATE TABLE s3_storages (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id        uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name           text NOT NULL,
    description    text NOT NULL DEFAULT '',
    endpoint       text NOT NULL, -- https://s3.eu-west-3.amazonaws.com, https://minio.example.com…
    region         text NOT NULL DEFAULT 'us-east-1',
    bucket         text NOT NULL,
    path_prefix    text NOT NULL DEFAULT '',
    access_key_enc text NOT NULL,
    secret_key_enc text NOT NULL,
    is_usable      boolean NOT NULL DEFAULT false,
    last_error     text NOT NULL DEFAULT '',
    checked_at     timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX s3_storages_team_name ON s3_storages (team_id, lower(name));

-- Sauvegardes planifiées : dump d'une base (kind database) ou archive d'un volume Docker (kind volume).
CREATE TABLE backup_schedules (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id       uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    kind          text NOT NULL CHECK (kind IN ('database', 'volume')),
    resource_id   uuid NOT NULL, -- base, application ou service
    volume_name   text NOT NULL DEFAULT '',
    databases     text NOT NULL DEFAULT '', -- bases à sauvegarder (séparées par des virgules), vide = base par défaut
    dump_all      boolean NOT NULL DEFAULT false,
    frequency     text NOT NULL DEFAULT '0 0 * * *',
    enabled       boolean NOT NULL DEFAULT true,
    keep_local    int NOT NULL DEFAULT 7,  -- nombre de sauvegardes locales conservées (0 = illimité)
    keep_local_days int NOT NULL DEFAULT 0, -- âge maximal (jours, 0 = illimité)
    local_backup  boolean NOT NULL DEFAULT true, -- false : fichier local supprimé après envoi S3
    s3_storage_id uuid REFERENCES s3_storages(id) ON DELETE SET NULL,
    keep_s3       int NOT NULL DEFAULT 30,
    keep_s3_days  int NOT NULL DEFAULT 0,
    last_run_at   timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX backup_schedules_resource_idx ON backup_schedules (resource_id);

CREATE TABLE backup_executions (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    schedule_id   uuid REFERENCES backup_schedules(id) ON DELETE CASCADE,
    team_id       uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    resource_id   uuid NOT NULL,
    server_id     uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    database_name text NOT NULL DEFAULT '',
    status        text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'success', 'failed')),
    filename      text NOT NULL DEFAULT '', -- chemin sur le serveur
    size          bigint NOT NULL DEFAULT 0,
    s3_storage_id uuid REFERENCES s3_storages(id) ON DELETE SET NULL,
    s3_key        text NOT NULL DEFAULT '',
    local_deleted boolean NOT NULL DEFAULT false,
    s3_deleted    boolean NOT NULL DEFAULT false,
    message       text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    finished_at   timestamptz
);
CREATE INDEX backup_executions_schedule_idx ON backup_executions (schedule_id, created_at DESC);
CREATE INDEX backup_executions_resource_idx ON backup_executions (resource_id, created_at DESC);

-- +goose Down
DROP TABLE backup_executions;
DROP TABLE backup_schedules;
DROP TABLE s3_storages;
