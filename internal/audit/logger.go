package audit

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Logger struct {
	pool *pgxpool.Pool
}

func NewLogger(pool *pgxpool.Pool) *Logger {
	if pool == nil {
		return nil
	}
	return &Logger{pool: pool}
}

func (l *Logger) Record(r *http.Request, user *auth.User, bucket, key, action, status, errMsg string) {
	if l == nil || l.pool == nil || action == "" {
		return
	}
	var uid, account *int64
	if user != nil {
		id, acc := user.ID, auth.AccountID(user)
		uid, account = &id, &acc
	}
	var ip, ua, errp *string
	if v := clientIP(r); v != "" {
		ip = &v
	}
	if r != nil {
		if v := r.Header.Get("User-Agent"); v != "" {
			ua = &v
		}
	}
	if errMsg != "" {
		errp = &errMsg
	}
	if status == "" {
		status = "success"
	}
	ctx := context.Background()
	if r != nil {
		ctx = r.Context()
	}
	_, _ = l.pool.Exec(ctx, `
		INSERT INTO audit_logs (
			tenant_user_id, account_id, bucket_name, object_key, action, source_ip, user_agent, status, error_message
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, uid, account, emptyToNil(bucket), emptyToNil(key), action, ip, ua, status, errp)
}

func clientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func emptyToNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
