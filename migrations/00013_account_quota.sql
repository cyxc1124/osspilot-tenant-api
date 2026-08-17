-- +goose Up
ALTER TABLE tenant_users
    ADD COLUMN quota_bytes BIGINT,
    ADD COLUMN object_limit BIGINT,
    ADD COLUMN daily_upload_bytes BIGINT;

-- +goose Down
ALTER TABLE tenant_users
    DROP COLUMN IF EXISTS daily_upload_bytes,
    DROP COLUMN IF EXISTS object_limit,
    DROP COLUMN IF EXISTS quota_bytes;
