package audit

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type Entry struct {
	ID           int64
	UserID       *int64
	Username     *string
	TenantID     *int64
	TenantName   *string
	BucketName   *string
	ObjectKey    *string
	Action       string
	SourceIP     *string
	UserAgent    *string
	Status       string
	ErrorMessage *string
	CreatedAt    time.Time
}

type Filter struct {
	AccountID   int64
	AccountName string
	Username    string
	TenantName  string
	UserID      *int64
	BucketName  string
	ObjectKey   string
	Action      string
	Status      string
	SourceIP    string
	Keyword     string
	AdminOnly   bool
	CreatedFrom *time.Time
	CreatedTo   *time.Time
}

var adminActions = []string{
	"user_create", "user_delete", "user_update", "modify_user_role",
	"bucket_create", "bucket_delete", "modify_bucket",
	"request_tenant_api_access", "create_application", "update_application", "delete_application",
	"create_access_key", "disable_access_key", "issue_sts_credentials",
}

func (s *Store) List(ctx context.Context, f Filter, page, pageSize int) ([]Entry, int, error) {
	where, args := f.where()
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs l LEFT JOIN tenant_users u ON u.id = l.tenant_user_id LEFT JOIN tenant_users a ON a.id = l.account_id WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count audit: %w", err)
	}
	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.pool.Query(ctx, `
		SELECT l.id, l.tenant_user_id, u.username, l.account_id, a.username, l.bucket_name, l.object_key,
			l.action, l.source_ip, l.user_agent, l.status, l.error_message, l.created_at
		FROM audit_logs l
		LEFT JOIN tenant_users u ON u.id = l.tenant_user_id
		LEFT JOIN tenant_users a ON a.id = l.account_id
		WHERE `+where+`
		ORDER BY l.created_at DESC, l.id DESC
		LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()
	items, err := scanEntries(rows)
	return items, total, err
}

func (s *Store) Export(ctx context.Context, f Filter) ([]Entry, error) {
	where, args := f.where()
	rows, err := s.pool.Query(ctx, `
		SELECT l.id, l.tenant_user_id, u.username, l.account_id, a.username, l.bucket_name, l.object_key,
			l.action, l.source_ip, l.user_agent, l.status, l.error_message, l.created_at
		FROM audit_logs l
		LEFT JOIN tenant_users u ON u.id = l.tenant_user_id
		LEFT JOIN tenant_users a ON a.id = l.account_id
		WHERE `+where+`
		ORDER BY l.created_at DESC, l.id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("export audit: %w", err)
	}
	defer rows.Close()
	return scanEntries(rows)
}

func (f Filter) where() (string, []any) {
	var parts []string
	var args []any
	add := func(cond string, v any) {
		args = append(args, v)
		parts = append(parts, cond+"$"+itoa(len(args)))
	}
	if f.AccountID != 0 {
		add("l.account_id = ", f.AccountID)
	}
	if f.AccountName != "" {
		add("a.username = ", f.AccountName)
	}
	if f.Username != "" {
		add("u.username ILIKE ", "%"+f.Username+"%")
	}
	if f.TenantName != "" {
		add("a.username ILIKE ", "%"+f.TenantName+"%")
	}
	if f.UserID != nil {
		add("l.tenant_user_id = ", *f.UserID)
	}
	if f.BucketName != "" {
		add("l.bucket_name = ", f.BucketName)
	}
	if f.ObjectKey != "" {
		add("l.object_key = ", f.ObjectKey)
	}
	if f.Action != "" {
		add("l.action = ", f.Action)
	}
	if f.Status != "" {
		add("l.status = ", f.Status)
	}
	if f.SourceIP != "" {
		add("l.source_ip = ", f.SourceIP)
	}
	if f.AdminOnly {
		args = append(args, adminActions)
		parts = append(parts, "l.action = ANY($"+itoa(len(args))+")")
	}
	if f.Keyword != "" {
		pat := "%" + f.Keyword + "%"
		args = append(args, pat)
		n := itoa(len(args))
		parts = append(parts, "(u.username ILIKE $"+n+" OR l.bucket_name ILIKE $"+n+" OR l.object_key ILIKE $"+n+" OR l.action ILIKE $"+n+")")
	}
	if f.CreatedFrom != nil {
		add("l.created_at >= ", *f.CreatedFrom)
	}
	if f.CreatedTo != nil {
		add("l.created_at <= ", *f.CreatedTo)
	}
	if len(parts) == 0 {
		return "TRUE", args
	}
	return strings.Join(parts, " AND "), args
}

type entryRows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanEntries(rows entryRows) ([]Entry, error) {
	var out []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.TenantID, &e.TenantName, &e.BucketName, &e.ObjectKey, &e.Action, &e.SourceIP, &e.UserAgent, &e.Status, &e.ErrorMessage, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func itoa(n int) string { return fmt.Sprintf("%d", n) }
