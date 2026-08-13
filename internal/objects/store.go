package objects

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) ListPrefix(ctx context.Context, bucketID int64, prefix, after string, limit int) ([]record, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT object_key, COALESCE(size, 0), content_type, etag, created_by,
			COALESCE(last_seen_at, updated_at)
		FROM object_records
		WHERE bucket_id = $1
		  AND object_key LIKE $2 ESCAPE '\'
		  AND object_key > $3
		  AND object_key NOT LIKE '.trash/%'
		  AND object_key <> '.trash/'
		ORDER BY object_key
		LIMIT $4`, bucketID, likePrefix(prefix), after, limit)
	if err != nil {
		return nil, fmt.Errorf("list objects: %w", err)
	}
	defer rows.Close()
	var out []record
	for rows.Next() {
		var rec record
		var last time.Time
		if err := rows.Scan(&rec.Key, &rec.Size, &rec.ContentType, &rec.ETag, &rec.UploadedBy, &last); err != nil {
			return nil, err
		}
		s := last.UTC().Format(time.RFC3339)
		rec.LastModified = &s
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, bucketID int64, key string) (*record, error) {
	var rec record
	var last, created, updated time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT r.object_key, COALESCE(r.size, 0), r.content_type, r.etag, r.storage_class, r.created_by,
			COALESCE(r.last_seen_at, r.updated_at), r.created_at, r.updated_at, u.username
		FROM object_records r
		LEFT JOIN tenant_users u ON u.id = r.created_by
		WHERE r.bucket_id = $1 AND r.object_key = $2`, bucketID, key,
	).Scan(
		&rec.Key, &rec.Size, &rec.ContentType, &rec.ETag, &rec.StorageClass, &rec.UploadedBy,
		&last, &created, &updated, &rec.Username,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	ls, cs, us := last.UTC().Format(time.RFC3339), created.UTC().Format(time.RFC3339), updated.UTC().Format(time.RFC3339)
	rec.LastModified, rec.CreatedAt, rec.UpdatedAt = &ls, &cs, &us
	return &rec, nil
}
