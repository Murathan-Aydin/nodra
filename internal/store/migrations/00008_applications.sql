-- +goose Up
CREATE TABLE applications (
    id                         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id                    uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    environment_id             uuid NOT NULL REFERENCES environments(id) ON DELETE RESTRICT,
    destination_id             uuid NOT NULL REFERENCES destinations(id) ON DELETE RESTRICT,
    name                       text NOT NULL,
    description                text NOT NULL DEFAULT '',
    status                     text NOT NULL DEFAULT 'stopped',
    -- Source du code
    git_repository             text NOT NULL DEFAULT '', -- owner/name (source) ou URL complète (public / deploy key)
    git_branch                 text NOT NULL DEFAULT 'main',
    git_commit_sha             text NOT NULL DEFAULT 'HEAD',
    source_id                  uuid REFERENCES git_sources(id) ON DELETE RESTRICT,
    private_key_id             uuid REFERENCES private_keys(id) ON DELETE RESTRICT,
    -- Construction
    build_pack                 text NOT NULL CHECK (build_pack IN ('nixpacks', 'dockerfile', 'dockercompose', 'dockerimage', 'static')),
    docker_image               text NOT NULL DEFAULT '',
    docker_image_tag           text NOT NULL DEFAULT '',
    static_image               text NOT NULL DEFAULT 'nginx:alpine',
    base_directory             text NOT NULL DEFAULT '/',
    publish_directory          text NOT NULL DEFAULT '',
    dockerfile                 text NOT NULL DEFAULT '', -- Dockerfile en ligne (sans dépôt)
    dockerfile_location        text NOT NULL DEFAULT '/Dockerfile',
    dockerfile_target          text NOT NULL DEFAULT '',
    docker_compose_location    text NOT NULL DEFAULT '/docker-compose.yaml',
    docker_compose_raw         text NOT NULL DEFAULT '',
    docker_compose_domains     jsonb NOT NULL DEFAULT '{}', -- service → domaines
    install_command            text NOT NULL DEFAULT '',
    build_command              text NOT NULL DEFAULT '',
    start_command              text NOT NULL DEFAULT '',
    watch_paths                text NOT NULL DEFAULT '',
    -- Réseau et routage
    ports_exposes              text NOT NULL DEFAULT '',
    ports_mappings             text NOT NULL DEFAULT '',
    domains                    text[] NOT NULL DEFAULT '{}',
    redirect                   text NOT NULL DEFAULT 'both' CHECK (redirect IN ('both', 'www', 'non-www')),
    preview_url_template       text NOT NULL DEFAULT '{{pr_id}}.{{domain}}',
    custom_labels              text NOT NULL DEFAULT '',
    custom_docker_run_options  text NOT NULL DEFAULT '',
    -- Commandes de déploiement
    pre_deployment_command     text NOT NULL DEFAULT '',
    pre_deployment_container   text NOT NULL DEFAULT '',
    post_deployment_command    text NOT NULL DEFAULT '',
    post_deployment_container  text NOT NULL DEFAULT '',
    -- Réglages structurés
    health_check               jsonb NOT NULL DEFAULT '{}',
    limits                     jsonb NOT NULL DEFAULT '{}',
    settings                   jsonb NOT NULL DEFAULT '{}',
    webhook_secret_enc         text NOT NULL DEFAULT '',
    config_hash                text NOT NULL DEFAULT '', -- empreinte de la config au dernier déploiement
    created_at                 timestamptz NOT NULL DEFAULT now(),
    updated_at                 timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX applications_team_idx ON applications (team_id);
CREATE INDEX applications_env_idx ON applications (environment_id);
CREATE INDEX applications_repo_idx ON applications (git_repository, git_branch);

-- Vue de toutes les ressources (applications ; bases et services s'y ajouteront).
CREATE VIEW resources AS
    SELECT id, team_id, environment_id, destination_id, name, 'application'::text AS type FROM applications;

-- Variables d'environnement des ressources (chiffrées). is_preview : valeur propre aux prévisualisations de PR.
CREATE TABLE resource_env_vars (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type text NOT NULL,
    resource_id   uuid NOT NULL,
    key           text NOT NULL,
    value_enc     text NOT NULL,
    is_buildtime  boolean NOT NULL DEFAULT false,
    is_runtime    boolean NOT NULL DEFAULT true,
    is_preview    boolean NOT NULL DEFAULT false,
    is_literal    boolean NOT NULL DEFAULT false,
    is_multiline  boolean NOT NULL DEFAULT false,
    is_secret     boolean NOT NULL DEFAULT false,
    comment       text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (resource_type, resource_id, key, is_preview)
);
CREATE INDEX resource_env_vars_resource_idx ON resource_env_vars (resource_id);

-- Stockages : volume Docker nommé, montage d'un dossier de l'hôte, ou fichier généré.
CREATE TABLE resource_storages (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    resource_type text NOT NULL,
    resource_id   uuid NOT NULL,
    kind          text NOT NULL CHECK (kind IN ('volume', 'bind', 'file')),
    name          text NOT NULL DEFAULT '', -- volume
    host_path     text NOT NULL DEFAULT '', -- bind
    mount_path    text NOT NULL,
    content       text NOT NULL DEFAULT '', -- file
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (resource_id, mount_path)
);

CREATE TABLE resource_tags (
    tag_id        uuid NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    resource_type text NOT NULL,
    resource_id   uuid NOT NULL,
    PRIMARY KEY (tag_id, resource_id)
);
CREATE INDEX resource_tags_resource_idx ON resource_tags (resource_id);

-- +goose Down
DROP TABLE resource_tags;
DROP TABLE resource_storages;
DROP TABLE resource_env_vars;
DROP VIEW resources;
DROP TABLE applications;
