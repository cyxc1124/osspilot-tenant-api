-- +goose Up
ALTER TABLE alert_events ADD COLUMN IF NOT EXISTS fingerprint VARCHAR(128);
CREATE UNIQUE INDEX IF NOT EXISTS ux_alert_events_fingerprint ON alert_events (fingerprint);

-- +goose Down
DROP INDEX IF EXISTS ux_alert_events_fingerprint;
ALTER TABLE alert_events DROP COLUMN IF EXISTS fingerprint;
