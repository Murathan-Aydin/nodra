-- +goose Up
CREATE TABLE projects (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX projects_team_name ON projects (team_id, lower(name));

CREATE TABLE environments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX environments_project_name ON environments (project_id, lower(name));

-- Variables partagées, référencées dans les ressources par {{team.CLÉ}}, {{project.CLÉ}}, {{environment.CLÉ}}, {{server.CLÉ}}.
CREATE TABLE shared_variables (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id      uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    scope_type   text NOT NULL CHECK (scope_type IN ('team', 'project', 'environment', 'server')),
    scope_id     uuid NOT NULL,
    key          text NOT NULL,
    value_enc    text NOT NULL,
    comment      text NOT NULL DEFAULT '',
    is_secret    boolean NOT NULL DEFAULT false, -- valeur jamais réaffichée après création
    is_multiline boolean NOT NULL DEFAULT false,
    is_literal   boolean NOT NULL DEFAULT false, -- pas d'interpolation de $VAR
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (scope_type, scope_id, key)
);
CREATE INDEX shared_variables_team_idx ON shared_variables (team_id);

CREATE TABLE tags (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name       text NOT NULL,
    color      text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX tags_team_name ON tags (team_id, lower(name));

-- +goose Down
DROP TABLE tags;
DROP TABLE shared_variables;
DROP TABLE environments;
DROP TABLE projects;
