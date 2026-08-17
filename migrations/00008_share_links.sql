-- +goose Up
CREATE TABLE share_links (
    id               BIGSERIAL PRIMARY KEY,
    account_id       BIGINT        NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    bucket_name      VARCHAR(255)  NOT NULL,
    object_key       VARCHAR(2048) NOT NULL,
    created_by       BIGINT        NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    token            VARCHAR(64)   NOT NULL,
    password_hash    VARCHAR(255),
    expires_at       TIMESTAMPTZ,
    max_access_count INTEGER,
    access_count     INTEGER       NOT NULL DEFAULT 0,
    allow_download   BOOLEAN       NOT NULL DEFAULT true,
    allow_preview    BOOLEAN       NOT NULL DEFAULT true,
    status           VARCHAR(32)   NOT NULL DEFAULT 'active',
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ix_share_links_token ON share_links (token);
CREATE INDEX ix_share_links_account_id ON share_links (account_id);
CREATE INDEX ix_share_links_created_by ON share_links (created_by);
CREATE INDEX ix_share_links_bucket_object ON share_links (bucket_name, object_key);
CREATE INDEX ix_share_links_status ON share_links (status);

-- +goose Down
DROP TABLE IF EXISTS share_links;
