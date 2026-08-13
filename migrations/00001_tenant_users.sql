-- +goose Up
CREATE TABLE tenant_users (
    id              BIGSERIAL PRIMARY KEY,
    username        VARCHAR(64)  NOT NULL,
    password_hash   VARCHAR(255) NOT NULL,
    display_name    VARCHAR(128),
    email           VARCHAR(255),
    phone           VARCHAR(32),
    status          VARCHAR(32)  NOT NULL DEFAULT 'active',
    role            VARCHAR(32)  NOT NULL DEFAULT 'normal_user',
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ix_tenant_users_username ON tenant_users (username);
CREATE INDEX ix_tenant_users_status ON tenant_users (status);

-- +goose Down
DROP TABLE IF EXISTS tenant_users;
