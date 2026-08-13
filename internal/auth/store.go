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
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (s *Store) GetByUsername(ctx context.Context, username string) (*User, error) {
	return s.scanUser(ctx, `
		SELECT id, username, password_hash, display_name, email, phone, status, role, must_change_password
		FROM tenant_users WHERE username = $1`, username)
}

func (s *Store) GetByID(ctx context.Context, id int64) (*User, error) {
	return s.scanUser(ctx, `
		SELECT id, username, password_hash, display_name, email, phone, status, role, must_change_password
		FROM tenant_users WHERE id = $1`, id)
}

func (s *Store) TouchLogin(ctx context.Context, id int64, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE tenant_users SET last_login_at = $2, updated_at = $2 WHERE id = $1`, id, at)
	return err
}

func (s *Store) UpdatePassword(ctx context.Context, id int64, hash string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE tenant_users SET password_hash = $2, must_change_password = false, updated_at = $3 WHERE id = $1`, id, hash, at)
	return err
}

func (s *Store) scanUser(ctx context.Context, q string, arg any) (*User, error) {
	var u User
	err := s.pool.QueryRow(ctx, q, arg).Scan(
		&u.ID, &u.Username, &u.PasswordHash, &u.DisplayName, &u.Email, &u.Phone, &u.Status, &u.Role, &u.MustChangePassword,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &u, nil
}
