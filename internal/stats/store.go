package stats

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Alert struct {
	ID         int64
	RuleType   string
	Severity   string
	Status     string
	Title      string
	Message    string
	BucketName *string
	FiredAt    time.Time
	ResolvedAt *time.Time
}

func (s *Store) ListAlerts(ctx context.Context, accountID int64, limit int) ([]Alert, error) {
	if s == nil || s.pool == nil {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, rule_type, severity, status, title, message, bucket_name, fired_at, resolved_at
		FROM alert_events
		WHERE notify_tenant = true AND account_id = $1 AND status IN ('firing', 'acknowledged')
		ORDER BY fired_at DESC
		LIMIT $2`, accountID, limit)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()
	var out []Alert
	for rows.Next() {
		var a Alert
		if err := rows.Scan(&a.ID, &a.RuleType, &a.Severity, &a.Status, &a.Title, &a.Message, &a.BucketName, &a.FiredAt, &a.ResolvedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
