-- +goose Up
-- ponytail: local copy until O4 projects ops system_settings here
CREATE TABLE platform_settings (
    key        VARCHAR(64) PRIMARY KEY,
    value      TEXT        NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS platform_settings;
