-- +goose Up
CREATE TABLE teams (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    parent_id   uuid REFERENCES teams(id) ON DELETE CASCADE, -- NULL = organisation (équipe racine)
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    personal    boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX teams_parent_idx ON teams (parent_id);

CREATE TABLE team_members (
    team_id    uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (team_id, user_id)
);
CREATE INDEX team_members_user_idx ON team_members (user_id);

-- Rôles système (team_id NULL, synchronisés depuis le code au démarrage) et rôles personnalisés.
CREATE TABLE roles (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     uuid REFERENCES teams(id) ON DELETE CASCADE,
    key         text NOT NULL DEFAULT '',
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    permissions text[] NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX roles_system_key ON roles (key) WHERE team_id IS NULL;
CREATE UNIQUE INDEX roles_team_name ON roles (team_id, lower(name)) WHERE team_id IS NOT NULL;

CREATE TABLE groups (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id     uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX groups_team_name ON groups (team_id, lower(name));

CREATE TABLE group_members (
    group_id   uuid NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (group_id, user_id)
);
CREATE INDEX group_members_user_idx ON group_members (user_id);

-- Attribution d'un rôle à un utilisateur ou un groupe sur une portée (équipe, projet, environnement, serveur, ressource).
CREATE TABLE role_assignments (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    user_id    uuid REFERENCES users(id) ON DELETE CASCADE,
    group_id   uuid REFERENCES groups(id) ON DELETE CASCADE,
    role_id    uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    scope_type text NOT NULL CHECK (scope_type IN ('team', 'project', 'environment', 'server', 'resource')),
    scope_id   uuid NOT NULL,
    effect     text NOT NULL DEFAULT 'allow' CHECK (effect IN ('allow', 'deny')),
    created_by uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((user_id IS NULL) <> (group_id IS NULL))
);
CREATE UNIQUE INDEX role_assignments_uniq ON role_assignments (coalesce(user_id, group_id), role_id, scope_type, scope_id, effect);
CREATE INDEX role_assignments_team_idx ON role_assignments (team_id);
CREATE INDEX role_assignments_user_idx ON role_assignments (user_id);
CREATE INDEX role_assignments_group_idx ON role_assignments (group_id);
CREATE INDEX role_assignments_scope_idx ON role_assignments (scope_type, scope_id);

CREATE TABLE team_invitations (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id    uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    email      text NOT NULL,
    role_id    uuid NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    group_ids  uuid[] NOT NULL DEFAULT '{}',
    token_hash text NOT NULL UNIQUE,
    invited_by uuid REFERENCES users(id) ON DELETE SET NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX team_invitations_email ON team_invitations (team_id, lower(email));

-- Journal d'audit en ajout seul (pas de clé étrangère : il survit à la suppression des objets).
CREATE TABLE audit_events (
    id             bigserial PRIMARY KEY,
    team_id        uuid,
    actor_user_id  uuid,
    actor_email    text NOT NULL DEFAULT '',
    actor_token_id uuid,
    action         text NOT NULL,
    target_type    text NOT NULL DEFAULT '',
    target_id      text NOT NULL DEFAULT '',
    ip             text NOT NULL DEFAULT '',
    user_agent     text NOT NULL DEFAULT '',
    metadata       jsonb NOT NULL DEFAULT '{}',
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_team_idx ON audit_events (team_id, id DESC);
CREATE INDEX audit_events_actor_idx ON audit_events (actor_user_id, id DESC);

-- +goose StatementBegin
CREATE FUNCTION audit_events_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_events est en ajout seul';
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
CREATE TRIGGER audit_events_no_update BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION audit_events_immutable();

CREATE TABLE approval_rules (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id            uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    name               text NOT NULL,
    scope_type         text NOT NULL CHECK (scope_type IN ('team', 'project', 'environment', 'server', 'resource')),
    scope_id           uuid NOT NULL,
    actions            text[] NOT NULL,
    required_approvals int NOT NULL DEFAULT 1 CHECK (required_approvals BETWEEN 1 AND 10),
    enabled            boolean NOT NULL DEFAULT true,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX approval_rules_team_idx ON approval_rules (team_id);

CREATE TABLE approval_requests (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id            uuid NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    rule_id            uuid REFERENCES approval_rules(id) ON DELETE SET NULL,
    required_approvals int NOT NULL,
    action             text NOT NULL,
    scope_type         text NOT NULL,
    scope_id           uuid NOT NULL,
    summary            text NOT NULL DEFAULT '',
    payload            jsonb NOT NULL DEFAULT '{}',
    requested_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    status             text NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled', 'executed', 'failed', 'expired')),
    error              text NOT NULL DEFAULT '',
    expires_at         timestamptz NOT NULL,
    decided_at         timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX approval_requests_team_idx ON approval_requests (team_id, status, created_at DESC);

CREATE TABLE approval_votes (
    request_id uuid NOT NULL REFERENCES approval_requests(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    decision   text NOT NULL CHECK (decision IN ('approve', 'reject')),
    comment    text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (request_id, user_id)
);

-- Les jetons API sont rattachés à une équipe (et à ses sous-équipes).
ALTER TABLE api_tokens ADD COLUMN team_id uuid REFERENCES teams(id) ON DELETE CASCADE;

-- +goose Down
ALTER TABLE api_tokens DROP COLUMN team_id;
DROP TABLE approval_votes;
DROP TABLE approval_requests;
DROP TABLE approval_rules;
DROP TABLE audit_events;
DROP FUNCTION audit_events_immutable();
DROP TABLE team_invitations;
DROP TABLE role_assignments;
DROP TABLE group_members;
DROP TABLE groups;
DROP TABLE roles;
DROP TABLE team_members;
DROP TABLE teams;
