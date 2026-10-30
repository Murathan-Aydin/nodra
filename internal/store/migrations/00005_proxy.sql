-- +goose Up
ALTER TABLE servers
    ADD COLUMN proxy_type       text NOT NULL DEFAULT 'traefik' CHECK (proxy_type IN ('traefik', 'caddy', 'none')),
    ADD COLUMN proxy_status     text NOT NULL DEFAULT 'stopped',
    ADD COLUMN proxy_compose    text NOT NULL DEFAULT '', -- configuration personnalisée (vide = générée)
    ADD COLUMN proxy_acme_email text NOT NULL DEFAULT '';

-- Fichiers de configuration dynamique du proxy (provider fichier Traefik ou import Caddy).
CREATE TABLE proxy_configs (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id  uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name       text NOT NULL,
    content    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (server_id, name)
);

-- Certificats TLS personnalisés (en plus de Let's Encrypt).
CREATE TABLE ssl_certificates (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name            text NOT NULL,
    domains         text[] NOT NULL,
    certificate     text NOT NULL,
    private_key_enc text NOT NULL,
    expires_at      timestamptz NOT NULL,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (server_id, name)
);

-- +goose Down
DROP TABLE ssl_certificates;
DROP TABLE proxy_configs;
ALTER TABLE servers DROP COLUMN proxy_type, DROP COLUMN proxy_status, DROP COLUMN proxy_compose, DROP COLUMN proxy_acme_email;
