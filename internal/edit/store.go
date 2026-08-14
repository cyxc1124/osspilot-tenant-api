package edit

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	lockTTL             = 2 * time.Hour
	sessionTTL          = 8 * time.Hour
	statusActive        = "active"
	statusClosed        = "closed"
	statusReleased      = "released"
	statusForceReleased = "force_released"
	editorMonaco        = "monaco"
	editorOffice        = "onlyoffice"
	modeEdit            = "edit"
	modeView            = "view"
)

var errLockTaken = errors.New("lock taken")

type Lock struct {
	ID        int64
	Bucket    string
	Key       string
	LockedBy  int64
	Token     string
	LockedAt  time.Time
	ExpiresAt time.Time
	Status    string
}

type Session struct {
	ID            string
	Bucket        string
	Key           string
	UserID        int64
	EditorType    string
	Mode          string
	DocumentKey   *string
	CallbackToken string
	Status        string
	LastSavedAt   *time.Time
	LastETag      *string
	CreatedAt     time.Time
	ExpiresAt     time.Time
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func newDocumentKey(bucket string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s_%s_%d", bucket, hex.EncodeToString(b), time.Now().Unix()), nil
}

func (s *Store) expireLocks(ctx context.Context, bucket, key string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE file_locks SET status = $4
		WHERE bucket_name = $1 AND object_key = $2 AND status = $3 AND expires_at <= now()`,
		bucket, key, statusActive, statusReleased)
	return err
}

func (s *Store) expireSessions(ctx context.Context, bucket, key string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE edit_sessions SET status = $4
		WHERE bucket_name = $1 AND object_key = $2 AND status = $3 AND expires_at <= now()`,
		bucket, key, statusActive, statusClosed)
	return err
}

func (s *Store) ActiveLock(ctx context.Context, bucket, key string) (*Lock, error) {
	if err := s.expireLocks(ctx, bucket, key); err != nil {
		return nil, fmt.Errorf("expire locks: %w", err)
	}
	lock, err := scanLock(s.pool.QueryRow(ctx, `
		SELECT id, bucket_name, object_key, locked_by, lock_token, locked_at, expires_at, status
		FROM file_locks
		WHERE bucket_name = $1 AND object_key = $2 AND status = $3 AND expires_at > now()
		ORDER BY locked_at DESC LIMIT 1`, bucket, key, statusActive))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get lock: %w", err)
	}
	return &lock, nil
}

func (s *Store) insertLock(ctx context.Context, bucket, key string, userID int64, token string, ttl time.Duration) (*Lock, error) {
	lock := Lock{Bucket: bucket, Key: key, LockedBy: userID, Token: token, Status: statusActive}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO file_locks (bucket_name, object_key, locked_by, lock_token, expires_at, status)
		VALUES ($1,$2,$3,$4, now() + ($5 * interval '1 second'), 'active')
		RETURNING id, locked_at, expires_at`,
		bucket, key, userID, token, int(ttl.Seconds()),
	).Scan(&lock.ID, &lock.LockedAt, &lock.ExpiresAt)
	if isUnique(err) {
		return nil, errLockTaken
	}
	if err != nil {
		return nil, fmt.Errorf("insert lock: %w", err)
	}
	return &lock, nil
}

func (s *Store) refreshLock(ctx context.Context, id int64, token *string, ttl time.Duration) (time.Time, error) {
	var expires time.Time
	err := s.pool.QueryRow(ctx, `
		UPDATE file_locks SET expires_at = now() + ($2 * interval '1 second'), lock_token = COALESCE($3, lock_token)
		WHERE id = $1 RETURNING expires_at`,
		id, int(ttl.Seconds()), token,
	).Scan(&expires)
	if err != nil {
		return time.Time{}, fmt.Errorf("refresh lock: %w", err)
	}
	return expires, nil
}

func (s *Store) releaseLock(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE file_locks SET status = $2 WHERE id = $1`, id, statusReleased)
	if err != nil {
		return fmt.Errorf("release lock: %w", err)
	}
	return nil
}

func (s *Store) closeSessionsForObject(ctx context.Context, bucket, key string) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE edit_sessions SET status = $4
		WHERE bucket_name = $1 AND object_key = $2 AND status = $3`,
		bucket, key, statusActive, statusClosed)
	if err != nil {
		return fmt.Errorf("close sessions: %w", err)
	}
	return nil
}

func (s *Store) ForceRelease(ctx context.Context, bucket, key string) (bool, error) {
	lock, err := s.ActiveLock(ctx, bucket, key)
	if err != nil {
		return false, err
	}
	if lock != nil {
		if _, err := s.pool.Exec(ctx, `UPDATE file_locks SET status = $2 WHERE id = $1`, lock.ID, statusForceReleased); err != nil {
			return false, fmt.Errorf("force release lock: %w", err)
		}
	}
	if err := s.closeSessionsForObject(ctx, bucket, key); err != nil {
		return false, err
	}
	return lock != nil, nil
}

func (s *Store) InsertSession(ctx context.Context, sess *Session) error {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO edit_sessions (
			id, bucket_name, object_key, user_id, editor_type, mode, document_key,
			callback_token, status, expires_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active',$9)
		RETURNING created_at`,
		sess.ID, sess.Bucket, sess.Key, sess.UserID, sess.EditorType, sess.Mode, sess.DocumentKey,
		sess.CallbackToken, sess.ExpiresAt,
	).Scan(&sess.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	sess.Status = statusActive
	return nil
}

func (s *Store) ActiveSession(ctx context.Context, id string) (*Session, error) {
	sess, err := scanSession(s.pool.QueryRow(ctx, `
		SELECT id, bucket_name, object_key, user_id, editor_type, mode, document_key,
			callback_token, status, last_saved_at, last_etag, created_at, expires_at
		FROM edit_sessions WHERE id = $1 AND status = $2 AND expires_at > now()`, id, statusActive))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &sess, nil
}

func (s *Store) SessionByDocumentKey(ctx context.Context, key string) (*Session, error) {
	sess, err := scanSession(s.pool.QueryRow(ctx, `
		SELECT id, bucket_name, object_key, user_id, editor_type, mode, document_key,
			callback_token, status, last_saved_at, last_etag, created_at, expires_at
		FROM edit_sessions WHERE document_key = $1 AND status = $2 AND expires_at > now()`, key, statusActive))
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get session by document: %w", err)
	}
	return &sess, nil
}

func (s *Store) CloseSession(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE edit_sessions SET status = $2 WHERE id = $1`, id, statusClosed)
	if err != nil {
		return fmt.Errorf("close session: %w", err)
	}
	return nil
}

func (s *Store) MarkSaved(ctx context.Context, id string, etag *string, at time.Time) error {
	_, err := s.pool.Exec(ctx, `UPDATE edit_sessions SET last_saved_at = $2, last_etag = $3 WHERE id = $1`, id, at, etag)
	if err != nil {
		return fmt.Errorf("mark session saved: %w", err)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanLock(row rowScanner) (Lock, error) {
	var lock Lock
	err := row.Scan(&lock.ID, &lock.Bucket, &lock.Key, &lock.LockedBy, &lock.Token, &lock.LockedAt, &lock.ExpiresAt, &lock.Status)
	return lock, err
}

func scanSession(row rowScanner) (Session, error) {
	var sess Session
	err := row.Scan(
		&sess.ID, &sess.Bucket, &sess.Key, &sess.UserID, &sess.EditorType, &sess.Mode, &sess.DocumentKey,
		&sess.CallbackToken, &sess.Status, &sess.LastSavedAt, &sess.LastETag, &sess.CreatedAt, &sess.ExpiresAt,
	)
	return sess, err
}

func isUnique(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23505"
}
