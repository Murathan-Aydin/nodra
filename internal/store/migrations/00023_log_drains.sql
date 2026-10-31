-- +goose Up
-- Log drains : Fluent Bit sur le serveur, configuration (secrets compris) chiffrée.
CREATE TABLE server_log_drains (
    server_id  uuid PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    type       text NOT NULL CHECK (type IN ('newrelic', 'axiom', 'custom')),
    config_enc text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE services ADD COLUMN log_drain_enabled boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE services DROP COLUMN log_drain_enabled;
DROP TABLE server_log_drains;
