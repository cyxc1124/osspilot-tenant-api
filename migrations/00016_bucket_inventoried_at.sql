-- +goose Up
ALTER TABLE buckets ADD COLUMN IF NOT EXISTS inventoried_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE buckets DROP COLUMN IF EXISTS inventoried_at;
