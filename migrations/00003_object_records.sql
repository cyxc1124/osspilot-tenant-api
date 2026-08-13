-- +goose Up
CREATE TABLE object_records (
    id            BIGSERIAL PRIMARY KEY,
    bucket_id     BIGINT       NOT NULL REFERENCES buckets (id) ON DELETE CASCADE,
    bucket_name   VARCHAR(63)  NOT NULL,
    object_key    VARCHAR(2048) NOT NULL,
    size          BIGINT,
    etag          VARCHAR(128),
    content_type  VARCHAR(255),
    storage_class VARCHAR(64),
    created_by    BIGINT REFERENCES tenant_users (id) ON DELETE SET NULL,
    updated_by    BIGINT REFERENCES tenant_users (id) ON DELETE SET NULL,
    last_seen_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX uq_object_records_bucket_key ON object_records (bucket_id, object_key);
CREATE INDEX ix_object_records_bucket_name ON object_records (bucket_name);

-- +goose Down
DROP TABLE IF EXISTS object_records;
