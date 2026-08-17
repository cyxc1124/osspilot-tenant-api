-- +goose Up
CREATE TABLE tenant_api_access (
    id            BIGSERIAL PRIMARY KEY,
    account_id    BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    status        VARCHAR(32) NOT NULL DEFAULT 'pending',
    requested_by  BIGINT REFERENCES tenant_users (id) ON DELETE SET NULL,
    requested_at  TIMESTAMPTZ,
    reviewed_at   TIMESTAMPTZ,
    review_note   TEXT,
    rgw_uid       VARCHAR(128),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ix_tenant_api_access_account_id ON tenant_api_access (account_id);

CREATE TABLE tenant_applications (
    id          BIGSERIAL PRIMARY KEY,
    account_id  BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    name        VARCHAR(128) NOT NULL,
    description TEXT,
    status      VARCHAR(32) NOT NULL DEFAULT 'active',
    created_by  BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_tenant_applications_account_name ON tenant_applications (account_id, name);
CREATE INDEX ix_tenant_applications_account_id ON tenant_applications (account_id);

CREATE TABLE application_access_keys (
    id                 BIGSERIAL PRIMARY KEY,
    application_id     BIGINT NOT NULL REFERENCES tenant_applications (id) ON DELETE CASCADE,
    account_id         BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    access_key_id      VARCHAR(64) NOT NULL,
    secret_key_hash    VARCHAR(255) NOT NULL,
    status             VARCHAR(32) NOT NULL DEFAULT 'active',
    description        VARCHAR(255),
    last_used_at       TIMESTAMPTZ,
    created_by         BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ix_application_access_keys_access_key_id ON application_access_keys (access_key_id);
CREATE INDEX ix_application_access_keys_application_id ON application_access_keys (application_id);

-- +goose Down
DROP TABLE IF EXISTS application_access_keys;
DROP TABLE IF EXISTS tenant_applications;
DROP TABLE IF EXISTS tenant_api_access;
