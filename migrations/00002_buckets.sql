-- +goose Up
CREATE TABLE buckets (
    id                       BIGSERIAL PRIMARY KEY,
    bucket_name              VARCHAR(63)  NOT NULL,
    display_name             VARCHAR(128),
    display_alias_only       BOOLEAN      NOT NULL DEFAULT false,
    quota_bytes              BIGINT,
    object_limit             BIGINT,
    versioning_enabled       BOOLEAN      NOT NULL DEFAULT false,
    access_logging_enabled   BOOLEAN      NOT NULL DEFAULT false,
    access_log_target_bucket VARCHAR(255),
    access_log_prefix        VARCHAR(512),
    status                   VARCHAR(32)  NOT NULL DEFAULT 'active',
    created_by               BIGINT REFERENCES tenant_users (id) ON DELETE SET NULL,
    created_at               TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at               TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX ix_buckets_bucket_name ON buckets (bucket_name);
CREATE INDEX ix_buckets_status ON buckets (status);

-- +goose Down
DROP TABLE IF EXISTS buckets;
