-- +goose Up
CREATE TABLE account_bucket_grants (
    user_id    BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    bucket_id  BIGINT NOT NULL REFERENCES buckets (id) ON DELETE CASCADE,
    local      BOOLEAN NOT NULL DEFAULT false,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, bucket_id)
);

CREATE INDEX ix_account_bucket_grants_bucket ON account_bucket_grants (bucket_id);

INSERT INTO account_bucket_grants (user_id, bucket_id, local)
SELECT created_by, id, true FROM buckets WHERE created_by IS NOT NULL
ON CONFLICT DO NOTHING;

-- +goose Down
DROP TABLE IF EXISTS account_bucket_grants;
