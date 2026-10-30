-- +goose Up
CREATE TABLE deployments (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id          uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    application_id   uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    server_id        uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    status           text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'in_progress', 'finished', 'failed', 'cancelled')),
    trigger          text NOT NULL DEFAULT 'manual', -- manual | webhook | api | rollback | restart | approval | preview
    branch           text NOT NULL DEFAULT '',
    commit_sha       text NOT NULL DEFAULT '',  -- commit demandé (vide = dernier de la branche)
    commit_message   text NOT NULL DEFAULT '',
    pr_number        int NOT NULL DEFAULT 0,
    force_rebuild    boolean NOT NULL DEFAULT false,
    rollback_image   text NOT NULL DEFAULT '',  -- image existante à redéployer (rollback, redémarrage)
    image            text NOT NULL DEFAULT '',  -- image effectivement déployée
    error            text NOT NULL DEFAULT '',
    cancel_requested boolean NOT NULL DEFAULT false,
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    started_at       timestamptz,
    finished_at      timestamptz
);
CREATE INDEX deployments_app_idx ON deployments (application_id, created_at DESC);
CREATE INDEX deployments_active_idx ON deployments (server_id, status) WHERE status IN ('queued', 'in_progress');

CREATE TABLE deployment_logs (
    id            bigserial PRIMARY KEY,
    deployment_id uuid NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now(),
    kind          text NOT NULL DEFAULT 'info', -- info | command | stdout | stderr | error
    hidden        boolean NOT NULL DEFAULT false, -- visible seulement en mode debug
    message       text NOT NULL
);
CREATE INDEX deployment_logs_dep_idx ON deployment_logs (deployment_id, id);

-- Prévisualisations de pull requests.
CREATE TABLE application_previews (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    application_id uuid NOT NULL REFERENCES applications(id) ON DELETE CASCADE,
    pr_number      int NOT NULL,
    pr_url         text NOT NULL DEFAULT '',
    title          text NOT NULL DEFAULT '',
    domains        text[] NOT NULL DEFAULT '{}',
    status         text NOT NULL DEFAULT 'stopped',
    last_commit    text NOT NULL DEFAULT '',
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (application_id, pr_number)
);

-- +goose Down
DROP TABLE application_previews;
DROP TABLE deployment_logs;
DROP TABLE deployments;
