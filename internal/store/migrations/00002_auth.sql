-- +goose Up
CREATE TABLE users (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email             text NOT NULL,
    name              text NOT NULL DEFAULT '',
    password_hash     text NOT NULL,
    is_instance_admin boolean NOT NULL DEFAULT false,
    email_verified_at timestamptz,
    totp_secret_enc   text,          -- secret TOTP chiffré (AES-GCM)
    totp_enabled_at   timestamptz,
    recovery_codes    text[] NOT NULL DEFAULT '{}', -- hash SHA-256 des codes restants
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE sessions (
    token_hash   text PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    mfa_pending  boolean NOT NULL DEFAULT false,
    ip           text NOT NULL DEFAULT '',
    user_agent   text NOT NULL DEFAULT '',
    expires_at   timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE api_tokens (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         text NOT NULL,
    token_hash   text NOT NULL UNIQUE,
    scopes       text[] NOT NULL DEFAULT '{}',
    expires_at   timestamptz,
    last_used_at timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX api_tokens_user_idx ON api_tokens (user_id);

CREATE TABLE password_resets (
    token_hash text PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE password_resets;
DROP TABLE api_tokens;
DROP TABLE sessions;
DROP TABLE users;
