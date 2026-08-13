package objects

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type Handler struct {
	buckets *bucket.Store
	store   *Store
	protect func(auth.UserHandler) http.HandlerFunc
}

func NewHandler(buckets *bucket.Store, store *Store, protect func(auth.UserHandler) http.HandlerFunc) *Handler {
	return &Handler{buckets: buckets, store: store, protect: protect}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/buckets/{bucket_name}/objects", h.protect(h.list))
	mux.HandleFunc("GET /api/buckets/{bucket_name}/objects/detail", h.protect(h.detail))
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

func (h *Handler) list(w http.ResponseWriter, r *http.Request, _ *auth.User) {
	b, ok := h.bucket(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	prefix := q.Get("prefix")
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

func (h *Handler) detail(w http.ResponseWriter, r *http.Request, _ *auth.User) {
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if key == "" || strings.HasPrefix(key, trashPrefix) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return
	}
	b, ok := h.bucket(w, r)
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

func (h *Handler) bucket(w http.ResponseWriter, r *http.Request) (*bucket.Bucket, bool) {
	b, err := h.buckets.GetByName(r.Context(), r.PathValue("bucket_name"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return nil, false
	}
	return b, true
}
