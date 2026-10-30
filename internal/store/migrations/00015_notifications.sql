-- +goose Up
CREATE TABLE notification_channels (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    type       text NOT NULL CHECK (type IN ('email', 'discord', 'slack', 'telegram', 'pushover', 'webhook')),
    name       text NOT NULL,
    config_enc text NOT NULL, -- configuration JSON chiffrée (URLs de webhook, jetons…)
    events     text[] NOT NULL DEFAULT '{}',
    enabled    boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notification_channels_team_idx ON notification_channels (team_id);

CREATE TABLE notification_deliveries (
    id         bigserial PRIMARY KEY,
    team_id    uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    channel_id uuid REFERENCES notification_channels(id) ON DELETE SET NULL,
    event      text NOT NULL,
    title      text NOT NULL DEFAULT '',
    status     text NOT NULL CHECK (status IN ('sent', 'failed')),
    error      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notification_deliveries_team_idx ON notification_deliveries (team_id, id DESC);

-- Limitation de fréquence (une alerte « disque plein » par jour et par serveur…).
CREATE TABLE notification_throttles (
    key     text PRIMARY KEY,
    sent_at timestamptz NOT NULL
);

-- E-mails de l'instance (réinitialisation, invitations, vérification, canal e-mail des équipes).
ALTER TABLE instance_settings ADD COLUMN mail_config_enc text NOT NULL DEFAULT '';

CREATE TABLE email_verifications (
    token_hash text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE email_verifications;
ALTER TABLE instance_settings DROP COLUMN mail_config_enc;
DROP TABLE notification_throttles;
DROP TABLE notification_deliveries;
DROP TABLE notification_channels;
