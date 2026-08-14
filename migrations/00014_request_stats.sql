-- +goose Up
CREATE TABLE platform_request_stats (
    period         VARCHAR(8) PRIMARY KEY,
    upload_bytes   BIGINT NOT NULL DEFAULT 0,
    download_bytes BIGINT NOT NULL DEFAULT 0,
    request_count  BIGINT NOT NULL DEFAULT 0,
    get_count      BIGINT NOT NULL DEFAULT 0,
    put_count      BIGINT NOT NULL DEFAULT 0,
    delete_count   BIGINT NOT NULL DEFAULT 0,
    error_count    BIGINT NOT NULL DEFAULT 0,
    active_users   BIGINT NOT NULL DEFAULT 0,
    collected_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE daily_platform_request_stats (
    stat_date      DATE PRIMARY KEY,
    upload_bytes   BIGINT NOT NULL DEFAULT 0,
    download_bytes BIGINT NOT NULL DEFAULT 0,
    request_count  BIGINT NOT NULL DEFAULT 0,
    get_count      BIGINT NOT NULL DEFAULT 0,
    put_count      BIGINT NOT NULL DEFAULT 0,
    delete_count   BIGINT NOT NULL DEFAULT 0,
    error_count    BIGINT NOT NULL DEFAULT 0,
    collected_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE account_request_stats (
    account_id     BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    period         VARCHAR(8) NOT NULL,
    upload_bytes   BIGINT NOT NULL DEFAULT 0,
    download_bytes BIGINT NOT NULL DEFAULT 0,
    request_count  BIGINT NOT NULL DEFAULT 0,
    get_count      BIGINT NOT NULL DEFAULT 0,
    put_count      BIGINT NOT NULL DEFAULT 0,
    delete_count   BIGINT NOT NULL DEFAULT 0,
    error_count    BIGINT NOT NULL DEFAULT 0,
    active_users   BIGINT NOT NULL DEFAULT 0,
    collected_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, period)
);

CREATE INDEX ix_account_request_stats_period ON account_request_stats (period);

CREATE TABLE bucket_request_stats (
    bucket_id      BIGINT NOT NULL REFERENCES buckets (id) ON DELETE CASCADE,
    period         VARCHAR(8) NOT NULL,
    account_id     BIGINT REFERENCES tenant_users (id) ON DELETE CASCADE,
    bucket_name    VARCHAR(255) NOT NULL,
    request_count  BIGINT NOT NULL DEFAULT 0,
    upload_bytes   BIGINT NOT NULL DEFAULT 0,
    download_bytes BIGINT NOT NULL DEFAULT 0,
    get_count      BIGINT NOT NULL DEFAULT 0,
    put_count      BIGINT NOT NULL DEFAULT 0,
    delete_count   BIGINT NOT NULL DEFAULT 0,
    collected_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (bucket_id, period)
);

CREATE INDEX ix_bucket_request_stats_account_period ON bucket_request_stats (account_id, period);

CREATE TABLE user_request_stats (
    user_id        BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    account_id     BIGINT NOT NULL REFERENCES tenant_users (id) ON DELETE CASCADE,
    period         VARCHAR(8) NOT NULL,
    username       VARCHAR(64),
    upload_count   BIGINT NOT NULL DEFAULT 0,
    download_count BIGINT NOT NULL DEFAULT 0,
    delete_count   BIGINT NOT NULL DEFAULT 0,
    access_count   BIGINT NOT NULL DEFAULT 0,
    upload_bytes   BIGINT NOT NULL DEFAULT 0,
    download_bytes BIGINT NOT NULL DEFAULT 0,
    collected_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, account_id, period)
);

CREATE INDEX ix_user_request_stats_account_period ON user_request_stats (account_id, period);

CREATE TABLE prefix_request_stats (
    bucket_id     BIGINT NOT NULL REFERENCES buckets (id) ON DELETE CASCADE,
    prefix        VARCHAR(1024) NOT NULL,
    period        VARCHAR(8) NOT NULL,
    account_id    BIGINT REFERENCES tenant_users (id) ON DELETE CASCADE,
    access_count  BIGINT NOT NULL DEFAULT 0,
    collected_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (bucket_id, prefix, period)
);

CREATE INDEX ix_prefix_request_stats_account_period ON prefix_request_stats (account_id, period);

-- +goose Down
DROP TABLE IF EXISTS prefix_request_stats;
DROP TABLE IF EXISTS user_request_stats;
DROP TABLE IF EXISTS bucket_request_stats;
DROP TABLE IF EXISTS account_request_stats;
DROP TABLE IF EXISTS daily_platform_request_stats;
DROP TABLE IF EXISTS platform_request_stats;
