package objects

import (
	"bytes"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/audit"
	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/queue"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

type Handler struct {
	buckets *bucket.Store
	store   *Store
	s3      *storage.Client
	protect func(auth.UserHandler) http.HandlerFunc
	ac      *rbac.Checker
	log     *audit.Logger
	q       *queue.Client
}

func NewHandler(buckets *bucket.Store, store *Store, s3 *storage.Client, protect func(auth.UserHandler) http.HandlerFunc, ac *rbac.Checker, log *audit.Logger, q *queue.Client) *Handler {
	return &Handler{buckets: buckets, store: store, s3: s3, protect: protect, ac: ac, log: log, q: q}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/buckets/{bucket_name}/objects", h.protect(h.list))
	mux.HandleFunc("GET /api/buckets/{bucket_name}/objects/detail", h.protect(h.detail))
	mux.HandleFunc("POST /api/buckets/{bucket_name}/objects/directories", h.protect(h.mkdir))
	mux.HandleFunc("DELETE /api/buckets/{bucket_name}/objects", h.protect(h.remove))
	mux.HandleFunc("GET /api/buckets/{bucket_name}/trash", h.protect(h.listTrash))
	mux.HandleFunc("POST /api/buckets/{bucket_name}/trash/restore", h.protect(h.restoreTrash))
	mux.HandleFunc("DELETE /api/buckets/{bucket_name}/trash", h.protect(h.purgeTrash))
	h.RegisterCopy(mux)
}

type summary struct {
	Key          string  `json:"key"`
	Size         int64   `json:"size"`
	ContentType  *string `json:"content_type"`
	LastModified *string `json:"last_modified"`
	ETag         *string `json:"etag"`
	UploadedBy   *int64  `json:"uploaded_by"`
}

type detail struct {
	Key                  string            `json:"key"`
	Size                 int64             `json:"size"`
	ContentType          *string           `json:"content_type"`
	LastModified         *string           `json:"last_modified"`
	ETag                 *string           `json:"etag"`
	StorageClass         *string           `json:"storage_class"`
	UploadedBy           *int64            `json:"uploaded_by"`
	UploadedByUsername   *string           `json:"uploaded_by_username"`
	CreatedAt            *string           `json:"created_at"`
	UpdatedAt            *string           `json:"updated_at"`
	AccessPermission     string            `json:"access_permission"`
	ServerSideEncryption *string           `json:"server_side_encryption"`
	UserMetadata         map[string]string `json:"user_metadata"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, user *auth.User) {
	q := r.URL.Query()
	prefix := q.Get("prefix")
	b, ok := h.bucket(w, r, user, rbac.ActionRead, prefix)
	if !ok {
		return
	}
	after := q.Get("continuation_token")
	maxKeys := defaultMaxKeys
	if raw := q.Get("max_keys"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "max_keys must be an integer")
			return
		}
		maxKeys = n
	}
	if maxKeys < 1 {
		maxKeys = 1
	}
	if maxKeys > maxListKeys {
		maxKeys = maxListKeys
	}
	recs, err := h.store.ListPrefix(r.Context(), b.ID, prefix, after, scanCap)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	items, prefixes, token, truncated := foldList(prefix, maxKeys, recs, len(recs) == scanCap)
	out := make([]summary, 0, len(items))
	for _, rec := range items {
		out = append(out, summary{
			Key: rec.Key, Size: rec.Size, ContentType: rec.ContentType,
			LastModified: rec.LastModified, ETag: rec.ETag, UploadedBy: rec.UploadedBy,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"items":              out,
		"prefixes":           prefixes,
		"is_truncated":       truncated,
		"continuation_token": token,
	})
}

func (h *Handler) detail(w http.ResponseWriter, r *http.Request, user *auth.User) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" || strings.HasPrefix(key, TrashPrefix) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return
	}
	b, ok := h.bucket(w, r, user, rbac.ActionRead, key)
	if !ok {
		return
	}
	rec, err := h.store.Get(r.Context(), b.ID, key)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if rec == nil {
		httpx.Error(w, http.StatusNotFound, "Object not found")
		return
	}
	httpx.JSON(w, http.StatusOK, detail{
		Key: rec.Key, Size: rec.Size, ContentType: rec.ContentType, LastModified: rec.LastModified,
		ETag: rec.ETag, StorageClass: rec.StorageClass, UploadedBy: rec.UploadedBy,
		UploadedByUsername: rec.Username, CreatedAt: rec.CreatedAt, UpdatedAt: rec.UpdatedAt,
		AccessPermission: "private", UserMetadata: map[string]string{},
	})
}

type mkdirReq struct {
	Name         string `json:"name"`
	ParentPrefix string `json:"parent_prefix"`
}

func (h *Handler) mkdir(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if h.s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return
	}
	var req mkdirReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	name := strings.TrimSpace(strings.Trim(req.Name, "/"))
	if name == "" || strings.Contains(name, "/") {
		httpx.Error(w, http.StatusBadRequest, "Directory name must not contain /")
		return
	}
	parent := strings.TrimSpace(req.ParentPrefix)
	if parent != "" && !strings.HasSuffix(parent, "/") {
		parent += "/"
	}
	key := parent + name + "/"
	if !ValidUserKey(key) {
		httpx.Error(w, http.StatusBadRequest, "Invalid directory key")
		return
	}
	b, ok := h.bucket(w, r, user, rbac.ActionWrite, key)
	if !ok {
		return
	}
	if _, err := h.s3.HeadObject(r.Context(), b.BucketName, key); err == nil {
		httpx.Error(w, http.StatusConflict, "Directory already exists")
		return
	} else if err != nil && !errors.Is(err, storage.ErrNotFound) {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	if _, err := h.s3.PutObject(r.Context(), b.BucketName, key, bytes.NewReader(nil), ""); err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	now := time.Now()
	if err := h.store.Upsert(r.Context(), b.ID, b.BucketName, key, 0, nil, nil, user.ID, now); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	ts := now.UTC().Format(time.RFC3339)
	httpx.JSON(w, http.StatusOK, map[string]any{"key": key, "size": 0, "last_modified": ts})
}

func (h *Handler) bucket(w http.ResponseWriter, r *http.Request, user *auth.User, action, prefix string) (*bucket.Bucket, bool) {
	b, err := h.buckets.GetVisible(r.Context(), auth.AccountID(user), r.PathValue("bucket_name"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return nil, false
	}
	if h.ac.Forbidden(w, r, user, b.BucketName, prefix, action) {
		return nil, false
	}
	return b, true
}

func (h *Handler) requireS3(w http.ResponseWriter) bool {
	if h.s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return false
	}
	return true
}
