package share

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var errTokenTaken = errors.New("share token taken")

type Link struct {
	ID             int64
	AccountID      int64
	BucketName     string
	ObjectKey      string
	CreatedBy      int64
	Token          string
	PasswordHash   *string
	ExpiresAt      *time.Time
	MaxAccessCount *int
	AccessCount    int
	AllowDownload  bool
	AllowPreview   bool
	Status         string
	CreatedAt      time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) Insert(ctx context.Context, link *Link) error {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO share_links (
			account_id, bucket_name, object_key, created_by, token, password_hash,
			expires_at, max_access_count, access_count, allow_download, allow_preview, status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,0,$9,$10,'active')
		RETURNING id, created_at, status, access_count`,
		link.AccountID, link.BucketName, link.ObjectKey, link.CreatedBy, link.Token, link.PasswordHash,
		link.ExpiresAt, link.MaxAccessCount, link.AllowDownload, link.AllowPreview,
	).Scan(&link.ID, &link.CreatedAt, &link.Status, &link.AccessCount)
	if isUnique(err) {
		return errTokenTaken
	}
	if err != nil {
		return fmt.Errorf("insert share: %w", err)
	}
	return nil
}

func (s *Store) List(ctx context.Context, accountID int64, bucketName, objectKey string) ([]Link, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, account_id, bucket_name, object_key, created_by, token, password_hash,
			expires_at, max_access_count, access_count, allow_download, allow_preview, status, created_at
		FROM share_links
		WHERE account_id = $1 AND status = 'active'
		  AND ($2 = '' OR bucket_name = $2)
		  AND ($3 = '' OR object_key = $3)
		ORDER BY created_at DESC, id DESC`, accountID, bucketName, objectKey)
	if err != nil {
		return nil, fmt.Errorf("list shares: %w", err)
	}
	defer rows.Close()
	var out []Link
	for rows.Next() {
		link, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, link)
	}
	return out, rows.Err()
}

func (s *Store) GetByID(ctx context.Context, id int64) (*Link, error) {
	link, err := scan(s.pool.QueryRow(ctx, `
		SELECT id, account_id, bucket_name, object_key, created_by, token, password_hash,
			expires_at, max_access_count, access_count, allow_download, allow_preview, status, created_at
		FROM share_links WHERE id = $1`, id))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get share: %w", err)
	}
	return &link, nil
}

func (s *Store) GetByToken(ctx context.Context, token string) (*Link, error) {
	link, err := scan(s.pool.QueryRow(ctx, `
		SELECT id, account_id, bucket_name, object_key, created_by, token, password_hash,
			expires_at, max_access_count, access_count, allow_download, allow_preview, status, created_at
		FROM share_links WHERE token = $1`, token))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get share by token: %w", err)
	}
	return &link, nil
}

func (s *Store) Revoke(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE share_links SET status = 'revoked' WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("revoke share: %w", err)
	}
	return nil
}

func (s *Store) BumpAccess(ctx context.Context, id int64, now time.Time) (int, bool, error) {
	var count int
	err := s.pool.QueryRow(ctx, `
		UPDATE share_links SET access_count = access_count + 1
		WHERE id = $1 AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > $2)
		  AND (max_access_count IS NULL OR access_count < max_access_count)
		RETURNING access_count`, id, now).Scan(&count)
	if err == pgx.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("bump share access: %w", err)
	}
	return count, true, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scan(row rowScanner) (Link, error) {
	var link Link
	err := row.Scan(
		&link.ID, &link.AccountID, &link.BucketName, &link.ObjectKey, &link.CreatedBy, &link.Token,
		&link.PasswordHash, &link.ExpiresAt, &link.MaxAccessCount, &link.AccessCount,
		&link.AllowDownload, &link.AllowPreview, &link.Status, &link.CreatedAt,
	)
	return link, err
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}
