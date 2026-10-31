-- +goose Up
-- Serveur MCP (outils pour assistants IA, par jeton API), désactivé par défaut.
ALTER TABLE instance_settings ADD COLUMN mcp_enabled boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE instance_settings DROP COLUMN mcp_enabled;
