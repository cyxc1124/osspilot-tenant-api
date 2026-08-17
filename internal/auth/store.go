package auth

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
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
	StorageRegionID    *int64
	StorageRegionCode  string
	StorageRegionName  string
	S3Endpoint         string
	S3RegionName       string
	CreatedAt          time.Time
	LastLoginAt        *time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const userCols = `u.id, u.username, u.password_hash, u.display_name, u.email, u.phone, u.status, u.role, u.must_change_password,
	COALESCE(u.account_id, u.id), u.quota_bytes, u.object_limit, u.daily_upload_bytes, u.created_at, u.last_login_at,
	acct.storage_region_id, acct.storage_region_code, acct.storage_region_name, acct.s3_endpoint, acct.s3_region_name`

const userFrom = `tenant_users u LEFT JOIN tenant_users acct ON acct.id = COALESCE(u.account_id, u.id)`

func (s *Store) GetByUsername(ctx context.Context, username string) (*User, error) {
	return s.scanUser(ctx, `SELECT `+userCols+` FROM `+userFrom+` WHERE u.username = $1`, username)
}

func (s *Store) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.scanUser(ctx, `SELECT `+userCols+` FROM `+userFrom+` WHERE u.id = $1`, id)
}

func (s *Store) BindS3(r *http.Request, accountID int64) *http.Request {
	if s == nil || r == nil || accountID < 1 {
		return r
	}
	u, err := s.GetByID(r.Context(), accountID)
	if err != nil || u == nil {
		return r
	}
	return r.WithContext(storage.WithAccountS3(r.Context(), u.S3Endpoint, u.S3RegionName))
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
			quota_bytes, object_limit, daily_upload_bytes,
			storage_region_id, storage_region_code, storage_region_name, s3_endpoint, s3_region_name,
			created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,'tenant_admin',COALESCE($7,true),$8,$9,$10,$11,$12,$13,$14,$15,now(),now())
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
			storage_region_id = EXCLUDED.storage_region_id,
			storage_region_code = EXCLUDED.storage_region_code,
			storage_region_name = EXCLUDED.storage_region_name,
			s3_endpoint = EXCLUDED.s3_endpoint,
			s3_region_name = EXCLUDED.s3_region_name,
			updated_at = now()
		RETURNING id`, u.Username, u.PasswordHash, u.DisplayName, u.Email, u.Phone, u.Status, u.MustChangePassword,
		u.QuotaBytes, u.ObjectLimit, u.DailyUploadBytes,
		u.StorageRegionID, u.StorageRegionCode, u.StorageRegionName, u.S3Endpoint, u.S3RegionName).Scan(&id)
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
	StorageRegionID    *int64
	StorageRegionCode  *string
	StorageRegionName  *string
	S3Endpoint         *string
	S3RegionName       *string
}

func (s *Store) ListByAccount(ctx context.Context, accountID int64) ([]User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT `+userCols+`
		FROM `+userFrom+`
		WHERE u.id = $1 OR COALESCE(u.account_id, u.id) = $1
		ORDER BY u.username`, accountID)
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
		FROM `+userFrom+`
		WHERE u.id = $2 AND (u.id = $1 OR COALESCE(u.account_id, u.id) = $1)`, accountID, userID)
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
	var code, name, endpoint, region sql.NullString
	err := row.Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.Phone, &u.Status, &u.Role, &u.MustChangePassword,
		&u.AccountID, &u.QuotaBytes, &u.ObjectLimit, &u.DailyUploadBytes, &u.CreatedAt, &u.LastLoginAt,
		&u.StorageRegionID, &code, &name, &endpoint, &region,
	)
	if err != nil {
		return u, err
	}
	u.StorageRegionCode = code.String
	u.StorageRegionName = name.String
	u.S3Endpoint = endpoint.String
	u.S3RegionName = region.String
	return u, nil
}
