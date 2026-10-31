-- +goose Up
-- Autorité de certification par serveur (certificats TLS des bases).
CREATE TABLE server_cas (
    server_id  uuid PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
    cert_pem   text NOT NULL,
    key_enc    text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE server_cas;
