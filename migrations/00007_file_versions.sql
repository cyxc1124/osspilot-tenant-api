-- +goose Up
CREATE TABLE file_versions (
    id           BIGSERIAL PRIMARY KEY,
    bucket_name  VARCHAR(255)  NOT NULL,
    object_key   VARCHAR(2048) NOT NULL,
    version_no   INTEGER       NOT NULL,
    storage_key  VARCHAR(2048) NOT NULL,
    size         BIGINT        NOT NULL,
    etag         VARCHAR(128),
    created_by   BIGINT        NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT now(),
    source       VARCHAR(32)   NOT NULL DEFAULT 'text_edit',
    remark       VARCHAR(512)
);

CREATE UNIQUE INDEX ix_file_versions_bucket_object_version
    ON file_versions (bucket_name, object_key, version_no);
CREATE INDEX ix_file_versions_created_by ON file_versions (created_by);

-- +goose Down
DROP TABLE IF EXISTS file_versions;
