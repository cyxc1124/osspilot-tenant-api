-- +goose Up
ALTER TABLE tenant_users
    ADD COLUMN storage_region_id BIGINT,
    ADD COLUMN storage_region_code TEXT,
    ADD COLUMN storage_region_name TEXT,
    ADD COLUMN s3_endpoint TEXT,
    ADD COLUMN s3_region_name TEXT;

-- +goose Down
ALTER TABLE tenant_users
    DROP COLUMN IF EXISTS s3_region_name,
    DROP COLUMN IF EXISTS s3_endpoint,
    DROP COLUMN IF EXISTS storage_region_name,
    DROP COLUMN IF EXISTS storage_region_code,
    DROP COLUMN IF EXISTS storage_region_id;
