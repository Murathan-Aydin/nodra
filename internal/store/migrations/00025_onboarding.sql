-- +goose Up
-- Configuration guidée proposée une seule fois, après la création du premier compte.
-- Les instances déjà utilisées (au moins un compte) la considèrent comme faite.
ALTER TABLE instance_settings ADD COLUMN onboarding_completed boolean NOT NULL DEFAULT false;
UPDATE instance_settings SET onboarding_completed = EXISTS (SELECT 1 FROM users);

-- +goose Down
ALTER TABLE instance_settings DROP COLUMN onboarding_completed;
