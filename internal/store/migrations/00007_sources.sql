-- +goose Up
CREATE TABLE git_sources (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id            uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    type               text NOT NULL CHECK (type IN ('github', 'gitlab', 'gitea')),
    name               text NOT NULL,
    html_url           text NOT NULL,
    api_url            text NOT NULL,
    organization       text NOT NULL DEFAULT '',
    is_public          boolean NOT NULL DEFAULT false,
    -- GitHub App
    app_id             bigint NOT NULL DEFAULT 0,
    app_slug           text NOT NULL DEFAULT '',
    client_id          text NOT NULL DEFAULT '',
    client_secret_enc  text NOT NULL DEFAULT '',
    webhook_secret_enc text NOT NULL DEFAULT '',
    private_key_enc    text NOT NULL DEFAULT '',
    installation_id    bigint NOT NULL DEFAULT 0,
    -- GitLab / Gitea
    access_token_enc   text NOT NULL DEFAULT '',
    -- Flux de création par manifeste (jeton d'état haché)
    state_hash         text NOT NULL DEFAULT '',
    state_expires_at   timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX git_sources_team_name ON git_sources (team_id, lower(name));
CREATE INDEX git_sources_app_idx ON git_sources (app_id) WHERE app_id > 0;
CREATE INDEX git_sources_state_idx ON git_sources (state_hash) WHERE state_hash <> '';

-- +goose Down
DROP TABLE git_sources;
