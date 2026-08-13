-- +goose Up
-- ponytail: 00002 once seeded admin; drop it if that revision already ran.
DELETE FROM tenant_users WHERE username = 'admin';

-- +goose Down
SELECT 1;
