-- +goose Up
CREATE TABLE file_locks (
    id          BIGSERIAL PRIMARY KEY,
    bucket_name VARCHAR(255)  NOT NULL,
    object_key  VARCHAR(2048) NOT NULL,
    locked_by   BIGINT        NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    lock_token  VARCHAR(64)   NOT NULL,
    locked_at   TIMESTAMPTZ   NOT NULL DEFAULT now(),
    expires_at  TIMESTAMPTZ   NOT NULL,
    status      VARCHAR(32)   NOT NULL DEFAULT 'active'
);

CREATE INDEX ix_file_locks_bucket_object ON file_locks (bucket_name, object_key);
CREATE INDEX ix_file_locks_locked_by ON file_locks (locked_by);
CREATE INDEX ix_file_locks_status ON file_locks (status);
CREATE UNIQUE INDEX uq_file_locks_active_object ON file_locks (bucket_name, object_key) WHERE status = 'active';

CREATE TABLE edit_sessions (
    id             VARCHAR(36)  PRIMARY KEY,
    bucket_name    VARCHAR(255)  NOT NULL,
    object_key     VARCHAR(2048) NOT NULL,
    user_id        BIGINT        NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    editor_type    VARCHAR(32)   NOT NULL,
    mode           VARCHAR(16)   NOT NULL DEFAULT 'edit',
    document_key   VARCHAR(128),
    callback_token VARCHAR(64)   NOT NULL DEFAULT '',
    status         VARCHAR(32)   NOT NULL DEFAULT 'active',
    last_saved_at  TIMESTAMPTZ,
    last_etag      VARCHAR(128),
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ   NOT NULL
);

CREATE INDEX ix_edit_sessions_bucket_object ON edit_sessions (bucket_name, object_key);
CREATE INDEX ix_edit_sessions_user_id ON edit_sessions (user_id);
CREATE INDEX ix_edit_sessions_status ON edit_sessions (status);
CREATE INDEX ix_edit_sessions_document_key ON edit_sessions (document_key);

-- +goose Down
DROP TABLE IF EXISTS edit_sessions;
DROP TABLE IF EXISTS file_locks;
