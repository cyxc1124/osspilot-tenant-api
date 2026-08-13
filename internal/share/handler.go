package share

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

type Handler struct {
	store   *Store
	buckets *bucket.Store
	s3      *storage.Client
	protect func(auth.UserHandler) http.HandlerFunc
	ac      *rbac.Checker
}

func NewHandler(store *Store, buckets *bucket.Store, s3 *storage.Client, protect func(auth.UserHandler) http.HandlerFunc, ac *rbac.Checker) *Handler {
	return &Handler{store: store, buckets: buckets, s3: s3, protect: protect, ac: ac}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/share-links", h.protect(h.create))
	mux.HandleFunc("GET /api/share-links", h.protect(h.list))
	mux.HandleFunc("DELETE /api/share-links/{link_id}", h.protect(h.revoke))
	mux.HandleFunc("GET /s/{token}", h.access)
}

type createReq struct {
	BucketName     string     `json:"bucket_name"`
	ObjectKey      string     `json:"object_key"`
	TenantID       *int64     `json:"tenant_id"`
	ExpiresAt      *time.Time `json:"expires_at"`
	Password       *string    `json:"password"`
	MaxAccessCount *int       `json:"max_access_count"`
	AllowDownload  *bool      `json:"allow_download"`
	AllowPreview   *bool      `json:"allow_preview"`
}

type itemJSON struct {
	ID             int64   `json:"id"`
	TenantID       int64   `json:"tenant_id"`
	BucketName     string  `json:"bucket_name"`
	ObjectKey      string  `json:"object_key"`
	CreatedBy      int64   `json:"created_by"`
	Token          string  `json:"token"`
	SharePath      string  `json:"share_path"`
	ExpiresAt      *string `json:"expires_at"`
	MaxAccessCount *int    `json:"max_access_count"`
	AccessCount    int     `json:"access_count"`
	AllowDownload  bool    `json:"allow_download"`
	AllowPreview   bool    `json:"allow_preview"`
	HasPassword    bool    `json:"has_password"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"created_at"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	if h.s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return
	}
	var req createReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !objects.ValidUserKey(req.ObjectKey) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return
	}
	allowDown, allowPrev := true, true
	if req.AllowDownload != nil {
		allowDown = *req.AllowDownload
	}
	if req.AllowPreview != nil {
		allowPrev = *req.AllowPreview
	}
	if !allowDown && !allowPrev {
		httpx.Error(w, http.StatusBadRequest, "At least one of allow_download or allow_preview must be true")
		return
	}
	if req.MaxAccessCount != nil && *req.MaxAccessCount < 1 {
		httpx.Error(w, http.StatusBadRequest, "max_access_count must be >= 1")
		return
	}
	var passwordHash *string
	if req.Password != nil {
		pw := strings.TrimSpace(*req.Password)
		if pw != "" {
			if len(pw) > 128 {
				httpx.Error(w, http.StatusBadRequest, "password must be 1-128 characters")
				return
			}
			hash, err := auth.HashPassword(pw)
			if err != nil {
				httpx.Error(w, http.StatusInternalServerError, "hash error")
				return
			}
			passwordHash = &hash
		}
	}
	now := time.Now().UTC()
	expires := req.ExpiresAt
	if expires == nil {
		t := now.Add(defaultShareTTL)
		expires = &t
	} else if !expires.After(now) {
		httpx.Error(w, http.StatusBadRequest, "expires_at must be in the future")
		return
	}
	b, ok := h.see(w, r, user, req.BucketName, req.ObjectKey, rbac.ActionShare)
	if !ok {
		return
	}
	if _, err := h.s3.HeadObject(r.Context(), b.BucketName, req.ObjectKey); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	link := &Link{
		AccountID:      auth.AccountID(user),
		BucketName:     b.BucketName,
		ObjectKey:      req.ObjectKey,
		CreatedBy:      user.ID,
		PasswordHash:   passwordHash,
		ExpiresAt:      expires,
		MaxAccessCount: req.MaxAccessCount,
		AllowDownload:  allowDown,
		AllowPreview:   allowPrev,
	}
	for range 3 {
		token, err := newToken()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "token error")
			return
		}
		link.Token = token
		err = h.store.Insert(r.Context(), link)
		if errors.Is(err, errTokenTaken) {
			continue
		}
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]any{"item": toItem(*link)})
		return
	}
	httpx.Error(w, http.StatusInternalServerError, "token error")
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	q := r.URL.Query()
	bucketName := q.Get("bucket_name")
	objectKey := q.Get("object_key")
	if bucketName != "" {
		b, ok := h.see(w, r, user, bucketName, objectKey, rbac.ActionShare)
		if !ok {
			return
		}
		bucketName = b.BucketName
	}
	rows, err := h.store.List(r.Context(), auth.AccountID(user), bucketName, objectKey)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	items := make([]itemJSON, 0, len(rows))
	for _, row := range rows {
		items = append(items, toItem(row))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	id, err := strconv.ParseInt(r.PathValue("link_id"), 10, 64)
	if err != nil || id < 1 {
		httpx.Error(w, http.StatusBadRequest, "invalid link_id")
		return
	}
	link, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if link == nil || link.Status != "active" {
		httpx.Error(w, http.StatusNotFound, "Share link not found")
		return
	}
	if link.AccountID != auth.AccountID(user) {
		httpx.Error(w, http.StatusForbidden, "Tenant access denied")
		return
	}
	if err := h.store.Revoke(r.Context(), link.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) access(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	if h.s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return
	}
	token := r.PathValue("token")
	if token == "" {
		httpx.Error(w, http.StatusNotFound, "Share link not found")
		return
	}
	link, err := h.store.GetByToken(r.Context(), token)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if link == nil || link.Status != "active" {
		httpx.Error(w, http.StatusNotFound, "Share link not found")
		return
	}
	now := time.Now().UTC()
	if link.ExpiresAt != nil && !link.ExpiresAt.After(now) {
		httpx.Error(w, http.StatusGone, "Share link has expired")
		return
	}
	if link.MaxAccessCount != nil && link.AccessCount >= *link.MaxAccessCount {
		httpx.Error(w, http.StatusForbidden, "Share link access limit reached")
		return
	}
	if link.PasswordHash != nil && *link.PasswordHash != "" {
		password := r.URL.Query().Get("password")
		if password == "" || !auth.CheckPassword(password, *link.PasswordHash) {
			httpx.Error(w, http.StatusUnauthorized, "Invalid or missing password")
			return
		}
	}
	if !link.AllowDownload && !link.AllowPreview {
		httpx.Error(w, http.StatusForbidden, "Download and preview are disabled for this share")
		return
	}
	if _, err := h.s3.HeadObject(r.Context(), link.BucketName, link.ObjectKey); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	ttl := presignTTL(link.ExpiresAt, now, h.s3.DownloadTTL())
	if ttl < time.Second {
		httpx.Error(w, http.StatusGone, "Share link has expired")
		return
	}
	var downloadURL, previewURL *string
	if link.AllowDownload {
		url, _, err := h.s3.PresignGetFor(r.Context(), link.BucketName, link.ObjectKey, ttl)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "storage error")
			return
		}
		downloadURL = &url
	}
	if link.AllowPreview {
		url, _, err := h.s3.PresignGetFor(r.Context(), link.BucketName, link.ObjectKey, ttl)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "storage error")
			return
		}
		previewURL = &url
	}
	// ponytail: skip CDN host rewrite (same as T4 downloads); platform_settings CDN is T5 read-only for now.
	count, ok, err := h.store.BumpAccess(r.Context(), link.ID, now)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if !ok {
		httpx.Error(w, http.StatusForbidden, "Share link access limit reached")
		return
	}
	expiresIn := int(ttl.Seconds())
	httpx.JSON(w, http.StatusOK, map[string]any{
		"bucket_name":      link.BucketName,
		"object_key":       link.ObjectKey,
		"allow_download":   link.AllowDownload,
		"allow_preview":    link.AllowPreview,
		"download_url":     downloadURL,
		"preview_url":      previewURL,
		"expires_at":       formatTime(link.ExpiresAt),
		"access_count":     count,
		"max_access_count": link.MaxAccessCount,
		"expires_in":       expiresIn,
	})
}

func toItem(link Link) itemJSON {
	return itemJSON{
		ID:             link.ID,
		TenantID:       link.AccountID,
		BucketName:     link.BucketName,
		ObjectKey:      link.ObjectKey,
		CreatedBy:      link.CreatedBy,
		Token:          link.Token,
		SharePath:      sharePath(link.Token),
		ExpiresAt:      formatTime(link.ExpiresAt),
		MaxAccessCount: link.MaxAccessCount,
		AccessCount:    link.AccessCount,
		AllowDownload:  link.AllowDownload,
		AllowPreview:   link.AllowPreview,
		HasPassword:    link.PasswordHash != nil && *link.PasswordHash != "",
		Status:         link.Status,
		CreatedAt:      link.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func (h *Handler) see(w http.ResponseWriter, r *http.Request, user *auth.User, name, key, action string) (*bucket.Bucket, bool) {
	b, err := h.buckets.GetVisible(r.Context(), auth.AccountID(user), name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return nil, false
	}
	if h.ac.Forbidden(w, r, user, b.BucketName, key, action) {
		return nil, false
	}
	return b, true
}
