-- +goose Up
-- Serveur « localhost » (l'hôte de l'instance) créé une seule fois, même s'il est supprimé ensuite.
ALTER TABLE instance_settings
    ADD COLUMN localhost_server_id    uuid REFERENCES servers(id) ON DELETE SET NULL,
    ADD COLUMN localhost_bootstrapped boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE instance_settings DROP COLUMN localhost_server_id, DROP COLUMN localhost_bootstrapped;
