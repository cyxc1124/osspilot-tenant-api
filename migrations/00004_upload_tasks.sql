-- +goose Up
CREATE TABLE upload_tasks (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT        NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    bucket_name  VARCHAR(63)   NOT NULL,
    object_key   VARCHAR(2048) NOT NULL,
    upload_type  VARCHAR(32)   NOT NULL DEFAULT 'simple',
    upload_id    VARCHAR(255),
    size         BIGINT,
    content_type VARCHAR(255),
    status       VARCHAR(32)   NOT NULL DEFAULT 'pending',
    created_at   TIMESTAMPTZ   NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    expired_at   TIMESTAMPTZ
);

CREATE INDEX ix_upload_tasks_user_id ON upload_tasks (user_id);
CREATE INDEX ix_upload_tasks_status ON upload_tasks (status);

-- +goose Down
DROP TABLE IF EXISTS upload_tasks;
