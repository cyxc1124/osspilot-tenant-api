package versions

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/audit"
	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/platform"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

type Handler struct {
	store    *Store
	buckets  *bucket.Store
	s3       *storage.Client
	s3fb     storage.Config
	settings *platform.Store
	protect  func(auth.UserHandler) http.HandlerFunc
	ac       *rbac.Checker
	log      *audit.Logger
	locked   func(ctx context.Context, bucket, key string) (bool, error)
}

func NewHandler(store *Store, buckets *bucket.Store, s3 *storage.Client, protect func(auth.UserHandler) http.HandlerFunc, ac *rbac.Checker, log *audit.Logger, settings *platform.Store, s3fb storage.Config, locked func(ctx context.Context, bucket, key string) (bool, error)) *Handler {
	return &Handler{store: store, buckets: buckets, s3: s3, s3fb: s3fb, settings: settings, protect: protect, ac: ac, log: log, locked: locked}
}

func (h *Handler) client(r *http.Request) *storage.Client {
	return storage.Live(r.Context(), h.s3fb, h.settings, h.s3)
}

func (h *Handler) live(w http.ResponseWriter, r *http.Request) *storage.Client {
	s3 := h.client(r)
	if s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return nil
	}
	return s3
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/versions", h.protect(h.list))
	mux.HandleFunc("GET /api/versions/{version_id}", h.protect(h.get))
	mux.HandleFunc("POST /api/versions/{version_id}/download", h.protect(h.download))
	mux.HandleFunc("POST /api/versions/{version_id}/restore", h.protect(h.restore))
	mux.HandleFunc("DELETE /api/versions/{version_id}", h.protect(h.remove))
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	q := r.URL.Query()
	bucketName := q.Get("bucket_name")
	objectKey := q.Get("object_key")
	if bucketName == "" || !objects.ValidUserKey(objectKey) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return
	}
	if !h.visible(w, r, user, bucketName) {
		return
	}
	limit, offset := defaultLimit, 0
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			httpx.Error(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if raw := q.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			httpx.Error(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = n
	}
	items, total, err := h.store.List(r.Context(), bucketName, objectKey, limit, offset)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, toJSON(item))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "total": total})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, user *auth.User) {
	v := h.load(w, r, user)
	if v == nil {
		return
	}
	httpx.JSON(w, http.StatusOK, toJSON(*v))
}

func (h *Handler) download(w http.ResponseWriter, r *http.Request, user *auth.User) {
	v := h.load(w, r, user)
	if v == nil {
		return
	}
	s3 := h.live(w, r)
	if s3 == nil {
		return
	}
	if _, err := s3.HeadObject(r.Context(), v.BucketName, v.StorageKey); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Version object not found in storage")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	url, expires, err := s3.PresignGet(r.Context(), v.BucketName, v.StorageKey)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	h.log.Record(r, user, v.BucketName, v.ObjectKey, "version_download", "success", "")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"download_url": url,
		"expires_in":   expires,
		"filename":     downloadName(v.ObjectKey, v.VersionNo),
	})
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request, user *auth.User) {
	v := h.load(w, r, user)
	if v == nil {
		return
	}
	s3 := h.live(w, r)
	if s3 == nil {
		return
	}
	if h.locked != nil {
		held, err := h.locked(r.Context(), v.BucketName, v.ObjectKey)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if held {
			httpx.Error(w, http.StatusConflict, "File is locked; cannot restore while another user is editing")
			return
		}
	}
	if _, err := s3.HeadObject(r.Context(), v.BucketName, v.StorageKey); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Version object not found in storage")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	_, err := s3.HeadObject(r.Context(), v.BucketName, v.ObjectKey)
	if err == nil {
		if err := h.archive(r, s3, user.ID, v.BucketName, v.ObjectKey, sourceRestore, "Auto-archived before restoring v"+strconv.Itoa(v.VersionNo)); err != nil {
			h.writeArchiveErr(w, err)
			return
		}
	} else if !errors.Is(err, storage.ErrNotFound) {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	etag, err := s3.CopyObject(r.Context(), v.BucketName, v.ObjectKey, v.BucketName, v.StorageKey)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	h.log.Record(r, user, v.BucketName, v.ObjectKey, "version_restore", "success", "")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"restored":    true,
		"bucket_name": v.BucketName,
		"object_key":  v.ObjectKey,
		"version_no":  v.VersionNo,
		"etag":        etag,
	})
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request, user *auth.User) {
	v := h.load(w, r, user)
	if v == nil {
		return
	}
	s3 := h.live(w, r)
	if s3 == nil {
		return
	}
	if err := s3.DeleteObject(r.Context(), v.BucketName, v.StorageKey); err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	if err := h.store.Delete(r.Context(), v.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	h.log.Record(r, user, v.BucketName, v.ObjectKey, "version_delete", "success", "")
	httpx.JSON(w, http.StatusOK, map[string]any{"deleted": true, "id": v.ID})
}

func (h *Handler) archive(r *http.Request, s3 *storage.Client, userID int64, bucketName, objectKey, source, remark string) error {
	var rem *string
	if remark != "" {
		rem = &remark
	}
	_, err := Archive(r.Context(), s3, h.store, userID, bucketName, objectKey, source, rem)
	return err
}

func (h *Handler) load(w http.ResponseWriter, r *http.Request, user *auth.User) *Record {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return nil
	}
	id, err := strconv.ParseInt(r.PathValue("version_id"), 10, 64)
	if err != nil || id < 1 {
		httpx.Error(w, http.StatusBadRequest, "invalid version_id")
		return nil
	}
	v, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil
	}
	if v == nil || !objects.ValidUserKey(v.ObjectKey) {
		httpx.Error(w, http.StatusNotFound, "Version not found")
		return nil
	}
	if !h.visible(w, r, user, v.BucketName) {
		return nil
	}
	return v
}

func (h *Handler) visible(w http.ResponseWriter, r *http.Request, user *auth.User, name string) bool {
	b, err := h.buckets.GetVisible(r.Context(), auth.AccountID(user), name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return false
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return false
	}
	return !h.ac.Forbidden(w, r, user, b.BucketName, "", rbac.ActionRead)
}

func (h *Handler) writeArchiveErr(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "Object not found")
		return
	}
	httpx.Error(w, http.StatusBadGateway, "storage error")
}

func toJSON(v Record) map[string]any {
	return map[string]any{
		"id":                  v.ID,
		"bucket_name":         v.BucketName,
		"object_key":          v.ObjectKey,
		"version_no":          v.VersionNo,
		"size":                v.Size,
		"etag":                v.ETag,
		"created_by":          v.CreatedBy,
		"created_by_username": v.CreatedByUsername,
		"created_at":          v.CreatedAt.UTC().Format(time.RFC3339),
		"source":              v.Source,
		"remark":              v.Remark,
	}
}
