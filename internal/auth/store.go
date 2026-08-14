package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type User struct {
	ID                 int64
	Username           string
	PasswordHash       string
	DisplayName        *string
	Email              *string
	Phone              *string
	Status             string
	Role               string
	MustChangePassword bool
	AccountID          int64
	QuotaBytes         *int64
	ObjectLimit        *int64
	DailyUploadBytes   *int64
	CreatedAt          time.Time
	LastLoginAt        *time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const userCols = `id, username, password_hash, display_name, email, phone, status, role, must_change_password,
	COALESCE(account_id, id), quota_bytes, object_limit, daily_upload_bytes, created_at, last_login_at`

func (s *Store) GetByUsername(ctx context.Context, username string) (*User, error) {
	return s.scanUser(ctx, `SELECT `+userCols+` FROM tenant_users WHERE username = $1`, username)
}

func (s *Store) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.scanUser(ctx, `SELECT `+userCols+` FROM tenant_users WHERE id = $1`, id)
}

func (s *Store) TouchLogin(ctx context.Context, id int64, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE tenant_users SET last_login_at = $2, updated_at = $2 WHERE id = $1`, id, at)
	return err
}

func (s *Store) UpdatePassword(ctx context.Context, id int64, hash string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE tenant_users SET password_hash = $2, must_change_password = false, updated_at = $3 WHERE id = $1`, id, hash, at)
	return err
}

func (s *Store) Upsert(ctx context.Context, u Upsert) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenant_users (
			username, password_hash, display_name, email, phone, status, role, must_change_password,
			quota_bytes, object_limit, daily_upload_bytes, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,'tenant_admin',COALESCE($7,true),$8,$9,$10,now(),now())
		ON CONFLICT (username) DO UPDATE SET
			password_hash = CASE WHEN EXCLUDED.password_hash <> '' THEN EXCLUDED.password_hash ELSE tenant_users.password_hash END,
			display_name = EXCLUDED.display_name,
			email = EXCLUDED.email,
			phone = EXCLUDED.phone,
			status = EXCLUDED.status,
			role = 'tenant_admin',
			must_change_password = COALESCE($7, tenant_users.must_change_password),
			quota_bytes = EXCLUDED.quota_bytes,
			object_limit = EXCLUDED.object_limit,
			daily_upload_bytes = EXCLUDED.daily_upload_bytes,
			updated_at = now()
		RETURNING id`, u.Username, u.PasswordHash, u.DisplayName, u.Email, u.Phone, u.Status, u.MustChangePassword,
		u.QuotaBytes, u.ObjectLimit, u.DailyUploadBytes).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("upsert user: %w", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE tenant_users SET account_id = id WHERE id = $1 AND account_id IS NULL`, id); err != nil {
		return 0, fmt.Errorf("set account_id: %w", err)
	}
	return id, nil
}

func (s *Store) DeleteByUsername(ctx context.Context, username string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tenant_users WHERE username = $1`, username)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

type Upsert struct {
	Username           string
	PasswordHash       string
	DisplayName        *string
	Email              *string
	Phone              *string
	Status             string
	MustChangePassword *bool
	QuotaBytes         *int64
	ObjectLimit        *int64
	DailyUploadBytes   *int64
}

func (s *Store) ListByAccount(ctx context.Context, accountID int64) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+userCols+`
		FROM tenant_users
		WHERE id = $1 OR COALESCE(account_id, id) = $1
		ORDER BY username`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUserRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) GetInAccount(ctx context.Context, accountID, userID int64) (*User, error) {
	return s.scanUser(ctx, `
		SELECT `+userCols+`
		FROM tenant_users
		WHERE id = $2 AND (id = $1 OR COALESCE(account_id, id) = $1)`, accountID, userID)
}

func (s *Store) InsertMember(ctx context.Context, accountID int64, username, hash string, display, email, phone *string, role string) (*User, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenant_users (
			username, password_hash, display_name, email, phone, status, role, account_id, must_change_password, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,'active',$6,$7,true,now(),now())
		RETURNING id`, username, hash, display, email, phone, role, accountID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetByID(ctx, id)
}

func (s *Store) UpdateMember(ctx context.Context, id int64, display, email, phone, status, role, hash *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tenant_users SET
			display_name = COALESCE($2, display_name),
			email = COALESCE($3, email),
			phone = COALESCE($4, phone),
			status = COALESCE($5, status),
			role = COALESCE($6, role),
			password_hash = COALESCE($7, password_hash),
			updated_at = now()
		WHERE id = $1`, id, display, email, phone, status, role, hash)
	return err
}

func (s *Store) DeleteInAccount(ctx context.Context, accountID, userID int64) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM tenant_users
		WHERE id = $2 AND COALESCE(account_id, id) = $1 AND id <> $1`, accountID, userID)
	return err
}

func (s *Store) scanUser(ctx context.Context, q string, args ...any) (*User, error) {
	u, err := scanUserRow(s.pool.QueryRow(ctx, q, args...))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}

type userRow interface {
	Scan(dest ...any) error
}

func scanUserRow(row userRow) (User, error) {
	var u User
	err := row.Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.Phone, &u.Status, &u.Role, &u.MustChangePassword,
		&u.AccountID, &u.QuotaBytes, &u.ObjectLimit, &u.DailyUploadBytes, &u.CreatedAt, &u.LastLoginAt,
	)
	return u, err
}
