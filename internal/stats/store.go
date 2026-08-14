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
	ID          int64
	RuleType    string
	Severity    string
	Status      string
	Title       string
	Message     string
	BucketName  *string
	FiredAt     time.Time
	ResolvedAt  *time.Time
	Fingerprint string
}

func (s *Store) AccountIDByUsername(ctx context.Context, username string) (int64, error) {
	if s == nil || s.pool == nil {
		return 0, nil
	}
	var id int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM tenant_users WHERE username = $1`, username).Scan(&id)
	if err != nil {
		return 0, nil
	}
	return id, nil
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

func (s *Store) UpsertAlert(ctx context.Context, accountID int64, a Alert) error {
	if s == nil || s.pool == nil || a.Fingerprint == "" {
		return nil
	}
	var fired any
	if !a.FiredAt.IsZero() {
		fired = a.FiredAt
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO alert_events (rule_type, severity, status, title, message, account_id, bucket_name, notify_tenant, fired_at, resolved_at, fingerprint)
		VALUES ($1,$2,$3,$4,$5,$6,$7,true,COALESCE($8, now()),$9,$10)
		ON CONFLICT (fingerprint) DO UPDATE SET
			status = EXCLUDED.status,
			title = EXCLUDED.title,
			message = EXCLUDED.message,
			severity = EXCLUDED.severity,
			bucket_name = EXCLUDED.bucket_name,
			resolved_at = EXCLUDED.resolved_at`,
		a.RuleType, a.Severity, a.Status, a.Title, a.Message, accountID, a.BucketName, fired, a.ResolvedAt, a.Fingerprint)
	if err != nil {
		return fmt.Errorf("upsert alert: %w", err)
	}
	return nil
}

func (s *Store) ResolveAlert(ctx context.Context, fingerprint string) error {
	if s == nil || s.pool == nil || fingerprint == "" {
		return nil
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE alert_events SET status='resolved', resolved_at=now()
		WHERE fingerprint = $1 AND status IN ('firing','acknowledged')`, fingerprint)
	if err != nil {
		return fmt.Errorf("resolve alert: %w", err)
	}
	return nil
}

func (s *Store) AuditWindow(ctx context.Context, since time.Time, actions []string) (total, failures int64, err error) {
	if s == nil || s.pool == nil || len(actions) == 0 {
		return 0, 0, nil
	}
	err = s.pool.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE status <> 'success')
		FROM audit_logs WHERE created_at >= $1 AND action = ANY($2)`, since, actions).Scan(&total, &failures)
	if err != nil {
		return 0, 0, fmt.Errorf("audit window: %w", err)
	}
	return total, failures, nil
}
