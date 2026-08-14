package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type UsageTotals struct {
	UsedBytes    int64
	ObjectCount  int64
	TrashBytes   int64
	TrashCount   int64
	VersionBytes int64
	VersionCount int64
}

type BucketUsage struct {
	BucketName  string
	UsedBytes   int64
	ObjectCount int64
	TrashBytes  int64
	TrashCount  int64
}

type StorageClassUsage struct {
	StorageClass string
	UsedBytes    int64
}

type UsageStore struct {
	pool *pgxpool.Pool
}

func NewUsageStore(pool *pgxpool.Pool) *UsageStore {
	return &UsageStore{pool: pool}
}

func (s *UsageStore) Totals(ctx context.Context) (UsageTotals, error) {
	var u UsageTotals
	err := s.pool.QueryRow(ctx, `
		SELECT
			COALESCE(SUM(size) FILTER (WHERE object_key NOT LIKE '.trash/%' AND object_key <> '.trash/' AND object_key NOT LIKE '.versions/%' AND object_key <> '.versions/'), 0),
			COUNT(*) FILTER (WHERE object_key NOT LIKE '.trash/%' AND object_key <> '.trash/' AND object_key NOT LIKE '.versions/%' AND object_key <> '.versions/'),
			COALESCE(SUM(size) FILTER (WHERE object_key LIKE '.trash/%' OR object_key = '.trash/'), 0),
			COUNT(*) FILTER (WHERE object_key LIKE '.trash/%' OR object_key = '.trash/'),
			COALESCE(SUM(size) FILTER (WHERE object_key LIKE '.versions/%' OR object_key = '.versions/'), 0),
			COUNT(*) FILTER (WHERE object_key LIKE '.versions/%' OR object_key = '.versions/')
		FROM object_records`).Scan(
		&u.UsedBytes, &u.ObjectCount, &u.TrashBytes, &u.TrashCount, &u.VersionBytes, &u.VersionCount,
	)
	if err != nil {
		return u, fmt.Errorf("usage totals: %w", err)
	}
	return u, nil
}

func (s *UsageStore) ByBucket(ctx context.Context) ([]BucketUsage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT bucket_name,
			COALESCE(SUM(size) FILTER (WHERE object_key NOT LIKE '.trash/%' AND object_key <> '.trash/' AND object_key NOT LIKE '.versions/%' AND object_key <> '.versions/'), 0),
			COUNT(*) FILTER (WHERE object_key NOT LIKE '.trash/%' AND object_key <> '.trash/' AND object_key NOT LIKE '.versions/%' AND object_key <> '.versions/'),
			COALESCE(SUM(size) FILTER (WHERE object_key LIKE '.trash/%' OR object_key = '.trash/'), 0),
			COUNT(*) FILTER (WHERE object_key LIKE '.trash/%' OR object_key = '.trash/')
		FROM object_records
		GROUP BY bucket_name
		ORDER BY 2 DESC`)
	if err != nil {
		return nil, fmt.Errorf("usage by bucket: %w", err)
	}
	defer rows.Close()
	var out []BucketUsage
	for rows.Next() {
		var b BucketUsage
		if err := rows.Scan(&b.BucketName, &b.UsedBytes, &b.ObjectCount, &b.TrashBytes, &b.TrashCount); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *UsageStore) ByStorageClass(ctx context.Context) ([]StorageClassUsage, *time.Time, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(storage_class, 'STANDARD'), COALESCE(SUM(size), 0), MAX(last_seen_at)
		FROM object_records
		WHERE object_key NOT LIKE '.trash/%' AND object_key <> '.trash/'
		  AND object_key NOT LIKE '.versions/%' AND object_key <> '.versions/'
		GROUP BY 1
		ORDER BY 2 DESC`)
	if err != nil {
		return nil, nil, fmt.Errorf("usage by storage class: %w", err)
	}
	defer rows.Close()
	var out []StorageClassUsage
	var collected *time.Time
	for rows.Next() {
		var u StorageClassUsage
		var seen *time.Time
		if err := rows.Scan(&u.StorageClass, &u.UsedBytes, &seen); err != nil {
			return nil, nil, err
		}
		out = append(out, u)
		if seen != nil && (collected == nil || seen.After(*collected)) {
			collected = seen
		}
	}
	return out, collected, rows.Err()
}
