package edit

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/platform"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
	"github.com/cyxc1124/osspilot-tenant-api/internal/versions"
)

const maxTextBytes = 2 * 1024 * 1024

type OfficeEnv struct {
	URL       string
	JWTSecret string
	PublicURL string
}

type Handler struct {
	store    *Store
	buckets  *bucket.Store
	versions *versions.Store
	settings *platform.Store
	s3       *storage.Client
	s3fb     storage.Config
	protect  func(auth.UserHandler) http.HandlerFunc
	office   OfficeEnv
	ac       *rbac.Checker
	secret   string
}

func NewHandler(store *Store, buckets *bucket.Store, versions *versions.Store, settings *platform.Store, s3 *storage.Client, protect func(auth.UserHandler) http.HandlerFunc, office OfficeEnv, ac *rbac.Checker, secret string, s3fb storage.Config) *Handler {
	office.URL = strings.TrimRight(office.URL, "/")
	office.PublicURL = strings.TrimRight(office.PublicURL, "/")
	if office.PublicURL == "" {
		office.PublicURL = "http://localhost:8000"
	}
	return &Handler{store: store, buckets: buckets, versions: versions, settings: settings, s3: s3, s3fb: s3fb, protect: protect, office: office, ac: ac, secret: secret}
}

func (h *Handler) client(r *http.Request) *storage.Client {
	return storage.Live(r.Context(), h.s3fb, h.settings, h.s3)
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/file-locks/status", h.protect(h.lockStatus))
	mux.HandleFunc("POST /api/file-locks/lock", h.protect(h.lockAcquire))
	mux.HandleFunc("POST /api/file-locks/refresh", h.protect(h.lockRefresh))
	mux.HandleFunc("POST /api/file-locks/unlock", h.protect(h.lockRelease))
	mux.HandleFunc("POST /api/text-edit/open", h.protect(h.textOpen))
	mux.HandleFunc("POST /api/text-edit/{session_id}/save", h.protect(h.textSave))
	mux.HandleFunc("POST /api/text-edit/{session_id}/presign-save", h.protect(h.textPresignSave))
	mux.HandleFunc("POST /api/text-edit/{session_id}/complete-save", h.protect(h.textCompleteSave))
	mux.HandleFunc("POST /api/text-edit/unlock", h.protect(h.textUnlock))
	mux.HandleFunc("POST /api/text-edit/close", h.protect(h.textClose))
	mux.HandleFunc("POST /api/editor/open", h.protect(h.officeOpen))
	mux.HandleFunc("GET /api/editor/files/{session_id}/download", h.officeDownload)
	mux.HandleFunc("POST /api/editor/callback/{session_id}", h.officeCallback)
	mux.HandleFunc("POST /api/editor/save", h.protect(h.officeSave))
	mux.HandleFunc("POST /api/editor/unlock", h.protect(h.officeUnlock))
	mux.HandleFunc("POST /internal/file-locks/force-unlock", h.forceUnlockInternal)
}

type objectReq struct {
	BucketName string  `json:"bucket_name"`
	ObjectKey  string  `json:"object_key"`
	TenantID   *int64  `json:"tenant_id"`
	LockToken  *string `json:"lock_token"`
}

type lockJSON struct {
	Locked    bool    `json:"locked"`
	LockToken *string `json:"lock_token"`
	LockedBy  *int64  `json:"locked_by"`
	ExpiresAt *string `json:"expires_at"`
	Readonly  bool    `json:"readonly"`
}

func (h *Handler) visible(w http.ResponseWriter, r *http.Request, user *auth.User, name, key string) (*bucket.Bucket, bool) {
	if !objects.ValidUserKey(key) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return nil, false
	}
	b, err := h.buckets.GetVisible(r.Context(), auth.AccountID(user), name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return nil, false
	}
	if h.ac.Forbidden(w, r, user, b.BucketName, key, rbac.ActionEdit) {
		return nil, false
	}
	return b, true
}

func (h *Handler) requireStore(w http.ResponseWriter) bool {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return false
	}
	return true
}

func (h *Handler) requireS3(w http.ResponseWriter) bool {
	if h.s3 == nil && !h.s3fb.Ready() {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return false
	}
	return true
}

func tokenOK(have, want string) bool {
	if len(have) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(have), []byte(want)) == 1
}

func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := formatTime(*t)
	return &s
}

func (h *Handler) acquire(r *http.Request, bucketName, key string, userID int64, token string) (*Lock, bool, error) {
	existing, err := h.store.ActiveLock(r.Context(), bucketName, key)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		if existing.LockedBy != userID {
			return existing, true, nil
		}
		tok := &token
		if token == "" {
			tok = nil
		}
		expires, err := h.store.refreshLock(r.Context(), existing.ID, tok, lockTTL)
		if err != nil {
			return nil, false, err
		}
		existing.ExpiresAt = expires
		if token != "" {
			existing.Token = token
		}
		return existing, false, nil
	}
	if token == "" {
		t, err := newToken()
		if err != nil {
			return nil, false, err
		}
		token = t
	}
	created, err := h.store.insertLock(r.Context(), bucketName, key, userID, token, lockTTL)
	if errors.Is(err, errLockTaken) {
		existing, err = h.store.ActiveLock(r.Context(), bucketName, key)
		if err != nil {
			return nil, false, err
		}
		if existing == nil {
			return nil, false, errors.New("lock race")
		}
		if existing.LockedBy != userID {
			return existing, true, nil
		}
		return existing, false, nil
	}
	return created, false, err
}

func (h *Handler) releaseOwned(r *http.Request, bucketName, key string, userID int64, token *string) (bool, error) {
	lock, err := h.store.ActiveLock(r.Context(), bucketName, key)
	if err != nil {
		return false, err
	}
	if lock == nil {
		return true, nil
	}
	if lock.LockedBy != userID {
		return false, nil
	}
	if token != nil && !tokenOK(lock.Token, *token) {
		return false, nil
	}
	if err := h.store.releaseLock(r.Context(), lock.ID); err != nil {
		return false, err
	}
	return true, nil
}

func lockResponse(lock *Lock, readonly bool) lockJSON {
	out := lockJSON{Readonly: readonly}
	if lock == nil {
		return out
	}
	out.Locked = !readonly
	out.LockedBy = &lock.LockedBy
	exp := formatTime(lock.ExpiresAt)
	out.ExpiresAt = &exp
	if !readonly {
		tok := lock.Token
		out.LockToken = &tok
	}
	return out
}
