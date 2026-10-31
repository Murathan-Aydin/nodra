-- +goose Up
CREATE TABLE oauth_providers (
    provider          text PRIMARY KEY,
    enabled           boolean NOT NULL DEFAULT false,
    client_id         text NOT NULL DEFAULT '',
    client_secret_enc text NOT NULL DEFAULT '',
    base_url          text NOT NULL DEFAULT '',
    tenant            text NOT NULL DEFAULT '',
    updated_at        timestamptz NOT NULL DEFAULT now()
);

-- Comptes externes liés aux utilisateurs.
CREATE TABLE user_identities (
    provider   text NOT NULL,
    subject    text NOT NULL,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, subject),
    UNIQUE (user_id, provider)
);

-- États du flux d'autorisation (usage unique, 10 minutes).
CREATE TABLE oauth_states (
    state_hash   text PRIMARY KEY,
    provider     text NOT NULL,
    verifier     text NOT NULL,
    redirect     text NOT NULL DEFAULT '/',
    link_user_id uuid REFERENCES users(id) ON DELETE CASCADE,
    expires_at   timestamptz NOT NULL
);

-- +goose Down
DROP TABLE oauth_states;
DROP TABLE user_identities;
DROP TABLE oauth_providers;
