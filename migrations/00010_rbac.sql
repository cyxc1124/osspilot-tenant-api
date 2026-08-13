-- +goose Up
ALTER TABLE tenant_users
    ADD COLUMN account_id BIGINT REFERENCES tenant_users (id) ON DELETE CASCADE;

UPDATE tenant_users SET account_id = id WHERE account_id IS NULL;

CREATE INDEX ix_tenant_users_account_id ON tenant_users (account_id);

CREATE TABLE tenant_roles (
    id          BIGSERIAL PRIMARY KEY,
    name        VARCHAR(64) NOT NULL,
    description VARCHAR(255),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ix_tenant_roles_name ON tenant_roles (name);

INSERT INTO tenant_roles (name, description) VALUES
    ('tenant_admin', 'Account administrator'),
    ('normal_user', 'Standard member'),
    ('readonly_user', 'Read-only member'),
    ('upload_user', 'Upload member'),
    ('audit_user', 'Audit member');

CREATE TABLE tenant_user_groups (
    id          BIGSERIAL PRIMARY KEY,
    account_id  BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    name        VARCHAR(64) NOT NULL,
    description VARCHAR(255),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_tenant_user_groups_account_name ON tenant_user_groups (account_id, name);
CREATE INDEX ix_tenant_user_groups_account_id ON tenant_user_groups (account_id);

CREATE TABLE tenant_user_group_members (
    id       BIGSERIAL PRIMARY KEY,
    group_id BIGINT NOT NULL REFERENCES tenant_user_groups (id) ON DELETE CASCADE,
    user_id  BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE
);

CREATE UNIQUE INDEX uq_tenant_user_group_members_group_user ON tenant_user_group_members (group_id, user_id);
CREATE INDEX ix_tenant_user_group_members_user_id ON tenant_user_group_members (user_id);

CREATE TABLE tenant_permissions (
    id         BIGSERIAL PRIMARY KEY,
    account_id BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    user_id    BIGINT REFERENCES tenant_users (id) ON DELETE CASCADE,
    role_id    BIGINT REFERENCES tenant_roles (id) ON DELETE CASCADE,
    group_id   BIGINT REFERENCES tenant_user_groups (id) ON DELETE CASCADE,
    bucket_id  BIGINT REFERENCES buckets (id) ON DELETE CASCADE,
    prefix     VARCHAR(1024),
    actions    JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT tenant_permissions_one_subject CHECK (
        (user_id IS NOT NULL)::int + (role_id IS NOT NULL)::int + (group_id IS NOT NULL)::int = 1
    )
);

CREATE INDEX ix_tenant_permissions_account_id ON tenant_permissions (account_id);
CREATE INDEX ix_tenant_permissions_user_id ON tenant_permissions (user_id);
CREATE INDEX ix_tenant_permissions_role_id ON tenant_permissions (role_id);
CREATE INDEX ix_tenant_permissions_group_id ON tenant_permissions (group_id);
CREATE INDEX ix_tenant_permissions_bucket_id ON tenant_permissions (bucket_id);

CREATE TABLE tenant_permission_templates (
    id          BIGSERIAL PRIMARY KEY,
    account_id  BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    name        VARCHAR(64) NOT NULL,
    description VARCHAR(255),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_tenant_permission_templates_account_name ON tenant_permission_templates (account_id, name);
CREATE INDEX ix_tenant_permission_templates_account_id ON tenant_permission_templates (account_id);

CREATE TABLE tenant_permission_template_rules (
    id          BIGSERIAL PRIMARY KEY,
    template_id BIGINT NOT NULL REFERENCES tenant_permission_templates (id) ON DELETE CASCADE,
    bucket_id   BIGINT REFERENCES buckets (id) ON DELETE CASCADE,
    prefix      VARCHAR(1024),
    actions     JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_tenant_permission_template_rules_template_id ON tenant_permission_template_rules (template_id);

CREATE TABLE tenant_permission_template_assignments (
    id          BIGSERIAL PRIMARY KEY,
    account_id  BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    template_id BIGINT NOT NULL REFERENCES tenant_permission_templates (id) ON DELETE CASCADE,
    user_id     BIGINT REFERENCES tenant_users (id) ON DELETE CASCADE,
    group_id    BIGINT REFERENCES tenant_user_groups (id) ON DELETE CASCADE,
    CONSTRAINT tenant_permission_template_assignments_one_subject CHECK (
        (user_id IS NOT NULL)::int + (group_id IS NOT NULL)::int = 1
    )
);

CREATE INDEX ix_tenant_permission_template_assignments_account_id ON tenant_permission_template_assignments (account_id);
CREATE INDEX ix_tenant_permission_template_assignments_template_id ON tenant_permission_template_assignments (template_id);

-- +goose Down
DROP TABLE IF EXISTS tenant_permission_template_assignments;
DROP TABLE IF EXISTS tenant_permission_template_rules;
DROP TABLE IF EXISTS tenant_permission_templates;
DROP TABLE IF EXISTS tenant_permissions;
DROP TABLE IF EXISTS tenant_user_group_members;
DROP TABLE IF EXISTS tenant_user_groups;
DROP TABLE IF EXISTS tenant_roles;
DROP INDEX IF EXISTS ix_tenant_users_account_id;
ALTER TABLE tenant_users DROP COLUMN IF EXISTS account_id;
