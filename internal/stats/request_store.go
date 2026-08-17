package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

type Traffic struct {
	UploadBytes, DownloadBytes int64
	RequestCount, GetCount     int64
	PutCount, DeleteCount      int64
	ErrorCount, ActiveUsers    int64
	CollectedAt                *time.Time
}

type BucketTraffic struct {
	BucketID                   int64
	RequestCount               int64
	UploadBytes, DownloadBytes int64
	GetCount, PutCount         int64
	DeleteCount                int64
	CollectedAt                *time.Time
}

type DailyTraffic struct {
	StatDate                   time.Time
	UploadBytes, DownloadBytes int64
	RequestCount, GetCount     int64
	PutCount, DeleteCount      int64
	ErrorCount                 int64
	CollectedAt                time.Time
}

type UserTraffic struct {
	UserID, AccountID          int64
	Username                   *string
	UploadCount, DownloadCount int64
	DeleteCount, AccessCount   int64
	UploadBytes, DownloadBytes int64
	CollectedAt                time.Time
}

type BucketRank struct {
	BucketID, AccountID        int64
	HasAccount                 bool
	BucketName                 string
	RequestCount               int64
	UploadBytes, DownloadBytes int64
	GetCount, PutCount         int64
	DeleteCount                int64
	CollectedAt                time.Time
}

type PrefixRank struct {
	BucketID, AccountID int64
	HasAccount          bool
	BucketName, Prefix  string
	AccessCount         int64
	CollectedAt         time.Time
}

func (s *Store) AccountTraffic(ctx context.Context, accountID int64, period string) (Traffic, error) {
	var t Traffic
	if s == nil || s.pool == nil {
		return t, nil
	}
	var collected time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT upload_bytes, download_bytes, request_count, get_count, put_count, delete_count, error_count, active_users, collected_at
		FROM account_request_stats WHERE account_id = $1 AND period = $2`, accountID, period).Scan(
		&t.UploadBytes, &t.DownloadBytes, &t.RequestCount, &t.GetCount, &t.PutCount, &t.DeleteCount, &t.ErrorCount, &t.ActiveUsers, &collected)
	if err == pgx.ErrNoRows {
		return t, nil
	}
	if err != nil {
		return t, fmt.Errorf("account traffic: %w", err)
	}
	t.CollectedAt = &collected
	return t, nil
}

func (s *Store) BucketTraffic(ctx context.Context, ids []int64, period string) (map[int64]BucketTraffic, error) {
	out := map[int64]BucketTraffic{}
	if s == nil || s.pool == nil || len(ids) == 0 {
		return out, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT bucket_id, request_count, upload_bytes, download_bytes, get_count, put_count, delete_count, collected_at
		FROM bucket_request_stats WHERE bucket_id = ANY($1) AND period = $2`, ids, period)
	if err != nil {
		return nil, fmt.Errorf("bucket traffic: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b BucketTraffic
		var collected time.Time
		if err := rows.Scan(&b.BucketID, &b.RequestCount, &b.UploadBytes, &b.DownloadBytes, &b.GetCount, &b.PutCount, &b.DeleteCount, &collected); err != nil {
			return nil, err
		}
		b.CollectedAt = &collected
		out[b.BucketID] = b
	}
	return out, rows.Err()
}

func (s *Store) PlatformTraffic(ctx context.Context, period string) (Traffic, error) {
	var t Traffic
	if s == nil || s.pool == nil {
		return t, nil
	}
	var collected time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT upload_bytes, download_bytes, request_count, get_count, put_count, delete_count, error_count, active_users, collected_at
		FROM platform_request_stats WHERE period = $1`, period).Scan(
		&t.UploadBytes, &t.DownloadBytes, &t.RequestCount, &t.GetCount, &t.PutCount, &t.DeleteCount, &t.ErrorCount, &t.ActiveUsers, &collected)
	if err == pgx.ErrNoRows {
		return t, nil
	}
	if err != nil {
		return t, fmt.Errorf("platform traffic: %w", err)
	}
	t.CollectedAt = &collected
	return t, nil
}

func (s *Store) DailyTraffic(ctx context.Context, days int) ([]DailyTraffic, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}
	if days < 1 {
		days = 30
	}
	rows, err := s.pool.Query(ctx, `
		SELECT stat_date, upload_bytes, download_bytes, request_count, get_count, put_count, delete_count, error_count, collected_at
		FROM daily_platform_request_stats ORDER BY stat_date DESC LIMIT $1`, days)
	if err != nil {
		return nil, fmt.Errorf("daily traffic: %w", err)
	}
	defer rows.Close()
	var rev []DailyTraffic
	for rows.Next() {
		var d DailyTraffic
		if err := rows.Scan(&d.StatDate, &d.UploadBytes, &d.DownloadBytes, &d.RequestCount, &d.GetCount, &d.PutCount, &d.DeleteCount, &d.ErrorCount, &d.CollectedAt); err != nil {
			return nil, err
		}
		rev = append(rev, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]DailyTraffic, 0, len(rev))
	for i := len(rev) - 1; i >= 0; i-- {
		out = append(out, rev[i])
	}
	return out, nil
}

func (s *Store) UserRanking(ctx context.Context, period, sortBy string, limit int) ([]UserTraffic, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}
	col := "access_count"
	switch sortBy {
	case "upload":
		col = "upload_count"
	case "download":
		col = "download_count"
	case "delete":
		col = "delete_count"
	}
	// ponytail: sort column is from a fixed switch, not request input.
	rows, err := s.pool.Query(ctx, `
		SELECT user_id, account_id, username, upload_count, download_count, delete_count, access_count, upload_bytes, download_bytes, collected_at
		FROM user_request_stats WHERE period = $1 ORDER BY `+col+` DESC LIMIT $2`, period, limit)
	if err != nil {
		return nil, fmt.Errorf("user ranking: %w", err)
	}
	defer rows.Close()
	var out []UserTraffic
	for rows.Next() {
		var u UserTraffic
		if err := rows.Scan(&u.UserID, &u.AccountID, &u.Username, &u.UploadCount, &u.DownloadCount, &u.DeleteCount, &u.AccessCount, &u.UploadBytes, &u.DownloadBytes, &u.CollectedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) BucketRanking(ctx context.Context, period string, limit int) ([]BucketRank, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT bucket_id, account_id, bucket_name, request_count, upload_bytes, download_bytes, get_count, put_count, delete_count, collected_at
		FROM bucket_request_stats WHERE period = $1 ORDER BY request_count DESC LIMIT $2`, period, limit)
	if err != nil {
		return nil, fmt.Errorf("bucket ranking: %w", err)
	}
	defer rows.Close()
	var out []BucketRank
	for rows.Next() {
		var b BucketRank
		var acc *int64
		if err := rows.Scan(&b.BucketID, &acc, &b.BucketName, &b.RequestCount, &b.UploadBytes, &b.DownloadBytes, &b.GetCount, &b.PutCount, &b.DeleteCount, &b.CollectedAt); err != nil {
			return nil, err
		}
		if acc != nil {
			b.AccountID, b.HasAccount = *acc, true
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) PrefixRanking(ctx context.Context, period string, limit int) ([]PrefixRank, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT p.bucket_id, p.account_id, COALESCE(b.bucket_name, ''), p.prefix, p.access_count, p.collected_at
		FROM prefix_request_stats p
		LEFT JOIN buckets b ON b.id = p.bucket_id
		WHERE p.period = $1 ORDER BY p.access_count DESC LIMIT $2`, period, limit)
	if err != nil {
		return nil, fmt.Errorf("prefix ranking: %w", err)
	}
	defer rows.Close()
	var out []PrefixRank
	for rows.Next() {
		var p PrefixRank
		var acc *int64
		if err := rows.Scan(&p.BucketID, &acc, &p.BucketName, &p.Prefix, &p.AccessCount, &p.CollectedAt); err != nil {
			return nil, err
		}
		if acc != nil {
			p.AccountID, p.HasAccount = *acc, true
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
