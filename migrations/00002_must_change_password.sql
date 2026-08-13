-- +goose Up
ALTER TABLE tenant_users
    ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE tenant_users DROP COLUMN IF EXISTS must_change_password;
