-- +goose Up
-- Consommation des conteneurs de chaque ressource (somme de ses conteneurs, hors prévisualisations),
-- relevée avec les métriques du serveur ; même rétention que celles-ci.
CREATE TABLE resource_metrics (
    resource_id uuid NOT NULL,
    server_id   uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    ts          timestamptz NOT NULL,
    cpu_percent real NOT NULL,
    mem_used    bigint NOT NULL,
    mem_limit   bigint NOT NULL,
    net_rx      bigint NOT NULL,
    net_tx      bigint NOT NULL,
    containers  int NOT NULL,
    PRIMARY KEY (resource_id, ts)
);
CREATE INDEX resource_metrics_server_ts ON resource_metrics (server_id, ts);

-- +goose Down
DROP TABLE resource_metrics;
