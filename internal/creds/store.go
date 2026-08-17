package creds

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type AccessRow struct {
	ID          int64
	AccountID   int64
	Username    string
	Status      string
	RequestedBy *int64
	RequestedAt *time.Time
	ReviewedAt  *time.Time
	ReviewNote  *string
	RGWUID      *string
	AppCount    int64
	KeyCount    int64
}

type App struct {
	ID          int64
	AccountID   int64
	Name        string
	Description *string
	Status      string
	CreatedBy   int64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	KeyCount    int64
}

type Key struct {
	ID            int64
	ApplicationID int64
	AccountID     int64
	AccessKeyID   string
	SecretHash    string
	Status        string
	Description   *string
	LastUsedAt    *time.Time
	CreatedBy     int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (s *Store) GetAccess(ctx context.Context, accountID int64) (*AccessRow, error) {
	return s.scanAccess(s.pool.QueryRow(ctx, accessSelect+` WHERE a.account_id = $1`, accountID))
}

func (s *Store) GetAccessByUsername(ctx context.Context, username string) (*AccessRow, error) {
	return s.scanAccess(s.pool.QueryRow(ctx, accessSelect+` WHERE u.username = $1`, username))
}

func (s *Store) GetAccessByRGWUID(ctx context.Context, uid string) (*AccessRow, error) {
	if strings.TrimSpace(uid) == "" {
		return nil, nil
	}
	return s.scanAccess(s.pool.QueryRow(ctx, accessSelect+` WHERE a.rgw_uid = $1 AND a.status = 'approved'`, uid))
}

func (s *Store) ListAccess(ctx context.Context, status string) ([]AccessRow, error) {
	q := accessSelect + ` WHERE ($1 = '' OR a.status = $1) ORDER BY a.requested_at DESC NULLS LAST, a.id DESC`
	rows, err := s.pool.Query(ctx, q, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AccessRow
	for rows.Next() {
		a, err := scanAccessRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) ReviewAccess(ctx context.Context, accountID int64, action string, note *string, rgwUID *string) (*AccessRow, error) {
	cur, err := s.GetAccess(ctx, accountID)
	if err != nil {
		return nil, err
	}
	if cur == nil {
		return nil, nil
	}
	next, err := reviewTarget(action, cur.Status)
	if err != nil {
		return nil, err
	}
	_, err = s.pool.Exec(ctx, `
		UPDATE tenant_api_access SET
			status = $2, reviewed_at = now(), review_note = $3,
			rgw_uid = COALESCE($4, rgw_uid), updated_at = now()
		WHERE account_id = $1`, accountID, next, note, rgwUID)
	if err != nil {
		return nil, err
	}
	return s.GetAccess(ctx, accountID)
}

const accessSelect = `
	SELECT a.id, a.account_id, u.username, a.status, a.requested_by, a.requested_at,
		a.reviewed_at, a.review_note, a.rgw_uid,
		(SELECT count(*) FROM tenant_applications t WHERE t.account_id = a.account_id),
		(SELECT count(*) FROM application_access_keys k WHERE k.account_id = a.account_id)
	FROM tenant_api_access a
	JOIN tenant_users u ON u.id = a.account_id`

func (s *Store) scanAccess(row pgx.Row) (*AccessRow, error) {
	a, err := scanAccessRow(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func scanAccessRow(row interface{ Scan(dest ...any) error }) (AccessRow, error) {
	var a AccessRow
	err := row.Scan(&a.ID, &a.AccountID, &a.Username, &a.Status, &a.RequestedBy, &a.RequestedAt,
		&a.ReviewedAt, &a.ReviewNote, &a.RGWUID, &a.AppCount, &a.KeyCount)
	return a, err
}

func (s *Store) RequestAccess(ctx context.Context, accountID, userID int64, note *string) (*AccessRow, error) {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO tenant_api_access (account_id, status, requested_by, requested_at, review_note)
		VALUES ($1,'pending',$2,now(),$3)
		ON CONFLICT (account_id) DO UPDATE SET
			status = 'pending',
			requested_by = EXCLUDED.requested_by,
			requested_at = now(),
			reviewed_at = NULL,
			review_note = EXCLUDED.review_note,
			updated_at = now()`, accountID, userID, note)
	if err != nil {
		return nil, err
	}
	return s.GetAccess(ctx, accountID)
}

func (s *Store) ListApps(ctx context.Context, accountID int64) ([]App, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT a.id, a.account_id, a.name, a.description, a.status, a.created_by, a.created_at, a.updated_at,
			(SELECT count(*) FROM application_access_keys k WHERE k.application_id = a.id)
		FROM tenant_applications a WHERE a.account_id = $1 ORDER BY a.created_at DESC`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []App
	for rows.Next() {
		var a App
		if err := rows.Scan(&a.ID, &a.AccountID, &a.Name, &a.Description, &a.Status, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt, &a.KeyCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) GetApp(ctx context.Context, accountID, id int64) (*App, error) {
	var a App
	err := s.pool.QueryRow(ctx, `
		SELECT a.id, a.account_id, a.name, a.description, a.status, a.created_by, a.created_at, a.updated_at,
			(SELECT count(*) FROM application_access_keys k WHERE k.application_id = a.id)
		FROM tenant_applications a WHERE a.id = $1 AND a.account_id = $2`, id, accountID).
		Scan(&a.ID, &a.AccountID, &a.Name, &a.Description, &a.Status, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt, &a.KeyCount)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) InsertApp(ctx context.Context, accountID, userID int64, name string, desc *string) (*App, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `
		INSERT INTO tenant_applications (account_id, name, description, created_by)
		VALUES ($1,$2,$3,$4) RETURNING id`, accountID, name, desc, userID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.GetApp(ctx, accountID, id)
}

func (s *Store) UpdateApp(ctx context.Context, accountID, id int64, name, desc, status *string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE tenant_applications SET
			name = COALESCE($3, name),
			description = COALESCE($4, description),
			status = COALESCE($5, status),
			updated_at = now()
		WHERE id = $1 AND account_id = $2`, id, accountID, name, desc, status)
	return err
}

func (s *Store) DeleteApp(ctx context.Context, accountID, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM tenant_applications WHERE id = $1 AND account_id = $2`, id, accountID)
	return err
}

func (s *Store) ListKeys(ctx context.Context, accountID, appID int64) ([]Key, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, application_id, account_id, access_key_id, secret_key_hash, status, description, last_used_at, created_at, updated_at
		FROM application_access_keys WHERE account_id = $1 AND application_id = $2 ORDER BY created_at DESC`, accountID, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Key
	for rows.Next() {
		k, err := scanKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

func (s *Store) InsertKey(ctx context.Context, k *Key) error {
	return s.pool.QueryRow(ctx, `
		INSERT INTO application_access_keys (
			application_id, account_id, access_key_id, secret_key_hash, description, created_by
		) VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, created_at, updated_at, status`,
		k.ApplicationID, k.AccountID, k.AccessKeyID, k.SecretHash, k.Description, k.CreatedBy).
		Scan(&k.ID, &k.CreatedAt, &k.UpdatedAt, &k.Status)
}

func (s *Store) GetKey(ctx context.Context, accountID, appID, id int64) (*Key, error) {
	k, err := scanKey(s.pool.QueryRow(ctx, `
		SELECT id, application_id, account_id, access_key_id, secret_key_hash, status, description, last_used_at, created_at, updated_at
		FROM application_access_keys WHERE id = $1 AND application_id = $2 AND account_id = $3`, id, appID, accountID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (s *Store) GetKeyByAccessID(ctx context.Context, accessKeyID string) (*Key, error) {
	k, err := scanKey(s.pool.QueryRow(ctx, `
		SELECT id, application_id, account_id, access_key_id, secret_key_hash, status, description, last_used_at, created_at, updated_at
		FROM application_access_keys WHERE access_key_id = $1`, accessKeyID))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &k, nil
}

func (s *Store) DisableKey(ctx context.Context, accountID, appID, id int64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE application_access_keys SET status = 'disabled', updated_at = now()
		WHERE id = $1 AND application_id = $2 AND account_id = $3`, id, appID, accountID)
	return err
}

func (s *Store) TouchKey(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE application_access_keys SET last_used_at = now() WHERE id = $1`, id)
	return err
}

type keyRow interface{ Scan(dest ...any) error }

func scanKey(row keyRow) (Key, error) {
	var k Key
	err := row.Scan(&k.ID, &k.ApplicationID, &k.AccountID, &k.AccessKeyID, &k.SecretHash, &k.Status, &k.Description, &k.LastUsedAt, &k.CreatedAt, &k.UpdatedAt)
	if err != nil {
		return Key{}, fmt.Errorf("scan key: %w", err)
	}
	return k, nil
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
