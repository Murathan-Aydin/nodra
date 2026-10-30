-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE instance_settings (
    id                   smallint PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    fqdn                 text,
    registration_enabled boolean NOT NULL DEFAULT true,
    auto_update_enabled  boolean NOT NULL DEFAULT true,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
INSERT INTO instance_settings (id) VALUES (1);

-- +goose Down
DROP TABLE instance_settings;
