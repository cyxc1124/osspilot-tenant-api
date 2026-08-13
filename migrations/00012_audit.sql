-- +goose Up
CREATE TABLE audit_logs (
    id              BIGSERIAL PRIMARY KEY,
    tenant_user_id  BIGINT REFERENCES tenant_users (id) ON DELETE SET NULL,
    account_id      BIGINT REFERENCES tenant_users (id) ON DELETE SET NULL,
    bucket_name     VARCHAR(255),
    object_key      VARCHAR(2048),
    action          VARCHAR(64) NOT NULL,
    source_ip       VARCHAR(64),
    user_agent      VARCHAR(512),
    status          VARCHAR(32) NOT NULL DEFAULT 'success',
    error_message   TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_audit_logs_account_id ON audit_logs (account_id);
CREATE INDEX ix_audit_logs_action ON audit_logs (action);
CREATE INDEX ix_audit_logs_created_at ON audit_logs (created_at);
CREATE INDEX ix_audit_logs_account_created ON audit_logs (account_id, created_at);
CREATE INDEX ix_audit_logs_bucket_object ON audit_logs (bucket_name, object_key);

-- ponytail: tenant read-only copy of ops-fired events; no alert_rules table until O-side engine.
CREATE TABLE alert_events (
    id            BIGSERIAL PRIMARY KEY,
    rule_type     VARCHAR(64) NOT NULL,
    severity      VARCHAR(32) NOT NULL,
    status        VARCHAR(32) NOT NULL DEFAULT 'firing',
    title         VARCHAR(255) NOT NULL,
    message       TEXT NOT NULL,
    account_id    BIGINT REFERENCES tenant_users (id) ON DELETE SET NULL,
    bucket_id     BIGINT REFERENCES buckets (id) ON DELETE SET NULL,
    bucket_name   VARCHAR(255),
    notify_tenant BOOLEAN NOT NULL DEFAULT false,
    fired_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at   TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX ix_alert_events_status ON alert_events (status);
CREATE INDEX ix_alert_events_account_id ON alert_events (account_id);
CREATE INDEX ix_alert_events_fired_at ON alert_events (fired_at);

-- +goose Down
DROP TABLE IF EXISTS alert_events;
DROP TABLE IF EXISTS audit_logs;
