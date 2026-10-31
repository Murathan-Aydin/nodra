-- +goose Up
-- Registres Docker privés (identifiants utilisés pour les pull, build et compose up sur les serveurs).
CREATE TABLE docker_registries (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id      uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name         text NOT NULL,
    host         text NOT NULL,
    username     text NOT NULL,
    password_enc text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (team_id, host)
);

-- +goose Down
DROP TABLE docker_registries;
