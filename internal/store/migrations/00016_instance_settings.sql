-- +goose Up
ALTER TABLE instance_settings
    ADD COLUMN instance_name          text NOT NULL DEFAULT 'Nodra',
    ADD COLUMN public_ipv4            text NOT NULL DEFAULT '',
    ADD COLUMN public_ipv6            text NOT NULL DEFAULT '',
    ADD COLUMN timezone               text NOT NULL DEFAULT 'UTC',
    ADD COLUMN auto_update_frequency  text NOT NULL DEFAULT '0 0 * * *',
    ADD COLUMN update_check_frequency text NOT NULL DEFAULT '0 * * * *',
    ADD COLUMN dns_validation_enabled boolean NOT NULL DEFAULT true,
    ADD COLUMN dns_servers            text[] NOT NULL DEFAULT '{}',
    ADD COLUMN api_enabled            boolean NOT NULL DEFAULT true,
    ADD COLUMN api_allowed_ips        text[] NOT NULL DEFAULT '{}',
    ADD COLUMN latest_version         text NOT NULL DEFAULT '',
    ADD COLUMN update_checked_at      timestamptz;

ALTER TABLE users ADD COLUMN disabled_at timestamptz;

-- +goose Down
ALTER TABLE users DROP COLUMN disabled_at;
ALTER TABLE instance_settings
    DROP COLUMN instance_name, DROP COLUMN public_ipv4, DROP COLUMN public_ipv6, DROP COLUMN timezone,
    DROP COLUMN auto_update_frequency, DROP COLUMN update_check_frequency, DROP COLUMN dns_validation_enabled,
    DROP COLUMN dns_servers, DROP COLUMN api_enabled, DROP COLUMN api_allowed_ips, DROP COLUMN latest_version,
    DROP COLUMN update_checked_at;
