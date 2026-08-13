package bucket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrConflict = errors.New("bucket exists")

type Bucket struct {
	ID                    int64
	BucketName            string
	DisplayName           *string
	DisplayAliasOnly      bool
	QuotaBytes            *int64
	ObjectLimit           *int64
	VersioningEnabled     bool
	AccessLoggingEnabled  bool
	AccessLogTargetBucket *string
	AccessLogPrefix       *string
	Status                string
	CreatedBy             *int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const bucketCols = `id, bucket_name, display_name, display_alias_only, quota_bytes, object_limit,
	versioning_enabled, access_logging_enabled, access_log_target_bucket, access_log_prefix,
	status, created_by, created_at, updated_at`

func (s *Store) List(ctx context.Context) ([]Bucket, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+bucketCols+` FROM buckets WHERE status = 'active' ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list buckets: %w", err)
	}
	defer rows.Close()
	var out []Bucket
	for rows.Next() {
		b, err := scanBucket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) GetByName(ctx context.Context, name string) (*Bucket, error) {
	b, err := scanBucket(s.pool.QueryRow(ctx, `SELECT `+bucketCols+` FROM buckets WHERE bucket_name = $1`, name))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get bucket: %w", err)
	}
	return &b, nil
}

func (s *Store) Insert(ctx context.Context, b *Bucket) error {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO buckets (
			bucket_name, display_name, display_alias_only, quota_bytes, object_limit,
			versioning_enabled, access_logging_enabled, access_log_target_bucket, access_log_prefix,
			status, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$12)
		RETURNING `+bucketCols, b.BucketName, b.DisplayName, b.DisplayAliasOnly, b.QuotaBytes, b.ObjectLimit,
		b.VersioningEnabled, b.AccessLoggingEnabled, b.AccessLogTargetBucket, b.AccessLogPrefix,
		b.Status, b.CreatedBy, b.CreatedAt,
	).Scan(
		&b.ID, &b.BucketName, &b.DisplayName, &b.DisplayAliasOnly, &b.QuotaBytes, &b.ObjectLimit,
		&b.VersioningEnabled, &b.AccessLoggingEnabled, &b.AccessLogTargetBucket, &b.AccessLogPrefix,
		&b.Status, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
	)
	if isUnique(err) {
		return ErrConflict
	}
	if err != nil {
		return fmt.Errorf("insert bucket: %w", err)
	}
	return nil
}

func (s *Store) Update(ctx context.Context, b *Bucket) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE buckets SET
			display_name = $2, display_alias_only = $3, quota_bytes = $4, object_limit = $5,
			versioning_enabled = $6, access_logging_enabled = $7, access_log_target_bucket = $8,
			access_log_prefix = $9, updated_at = $10
		WHERE id = $1`,
		b.ID, b.DisplayName, b.DisplayAliasOnly, b.QuotaBytes, b.ObjectLimit,
		b.VersioningEnabled, b.AccessLoggingEnabled, b.AccessLogTargetBucket, b.AccessLogPrefix, b.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update bucket: %w", err)
	}
	return nil
}

func (s *Store) Delete(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM buckets WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete bucket: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanBucket(row rowScanner) (Bucket, error) {
	var b Bucket
	err := row.Scan(
		&b.ID, &b.BucketName, &b.DisplayName, &b.DisplayAliasOnly, &b.QuotaBytes, &b.ObjectLimit,
		&b.VersioningEnabled, &b.AccessLoggingEnabled, &b.AccessLogTargetBucket, &b.AccessLogPrefix,
		&b.Status, &b.CreatedBy, &b.CreatedAt, &b.UpdatedAt,
	)
	return b, err
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}
