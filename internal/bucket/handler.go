package bucket

import (
	"errors"
	"net/http"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

type Handler struct {
	store   *Store
	s3      *storage.Client
	protect func(auth.UserHandler) http.HandlerFunc
}

func NewHandler(store *Store, protect func(auth.UserHandler) http.HandlerFunc, s3 *storage.Client) *Handler {
	return &Handler{store: store, protect: protect, s3: s3}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/buckets", h.protect(h.list))
	mux.HandleFunc("POST /api/buckets", h.protect(h.create))
	mux.HandleFunc("GET /api/buckets/{bucket_name}", h.protect(h.get))
	mux.HandleFunc("PUT /api/buckets/{bucket_name}", h.protect(h.update))
	mux.HandleFunc("DELETE /api/buckets/{bucket_name}", h.protect(h.remove))
}

type createRequest struct {
	BucketName            string  `json:"bucket_name"`
	DisplayName           *string `json:"display_name"`
	QuotaBytes            *int64  `json:"quota_bytes"`
	ObjectLimit           *int64  `json:"object_limit"`
	VersioningEnabled     bool    `json:"versioning_enabled"`
	AccessLoggingEnabled  bool    `json:"access_logging_enabled"`
	AccessLogTargetBucket *string `json:"access_log_target_bucket"`
	AccessLogPrefix       *string `json:"access_log_prefix"`
}

type updateRequest struct {
	DisplayName           *string `json:"display_name"`
	DisplayAliasOnly      *bool   `json:"display_alias_only"`
	QuotaBytes            *int64  `json:"quota_bytes"`
	ObjectLimit           *int64  `json:"object_limit"`
	VersioningEnabled     *bool   `json:"versioning_enabled"`
	AccessLoggingEnabled  *bool   `json:"access_logging_enabled"`
	AccessLogTargetBucket *string `json:"access_log_target_bucket"`
	AccessLogPrefix       *string `json:"access_log_prefix"`
}

type summary struct {
	BucketName        string  `json:"bucket_name"`
	DisplayName       *string `json:"display_name"`
	DisplayAliasOnly  bool    `json:"display_alias_only"`
	QuotaBytes        *int64  `json:"quota_bytes"`
	UsedBytes         int64   `json:"used_bytes"` // ponytail: 0 until T13
	ObjectCount       int64   `json:"object_count"`
	Status            string  `json:"status"`
	VersioningEnabled bool    `json:"versioning_enabled"`
	CreatedAt         string  `json:"created_at"`
}

type detail struct {
	ID                    int64   `json:"id"`
	BucketName            string  `json:"bucket_name"`
	DisplayName           *string `json:"display_name"`
	DisplayAliasOnly      bool    `json:"display_alias_only"`
	QuotaBytes            *int64  `json:"quota_bytes"`
	ObjectLimit           *int64  `json:"object_limit"`
	UsedBytes             int64   `json:"used_bytes"`
	ObjectCount           int64   `json:"object_count"`
	VersioningEnabled     bool    `json:"versioning_enabled"`
	AccessLoggingEnabled  bool    `json:"access_logging_enabled"`
	AccessLogTargetBucket *string `json:"access_log_target_bucket"`
	AccessLogPrefix       *string `json:"access_log_prefix"`
	Status                string  `json:"status"`
	CreatedAt             string  `json:"created_at"`
	UpdatedAt             string  `json:"updated_at"`
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, _ *auth.User) {
	items, err := h.store.List(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]summary, 0, len(items))
	for _, b := range items {
		out = append(out, toSummary(b))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, user *auth.User) {
	var req createRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := validateName(req.BucketName); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.validateLogging(w, r, req.AccessLoggingEnabled, req.AccessLogTargetBucket, req.BucketName); err != nil {
		return
	}
	now := time.Now()
	uid := user.ID
	// ponytail: RGW create happens below when S3 is configured.
	b := &Bucket{
		BucketName:            req.BucketName,
		DisplayName:           emptyToNil(req.DisplayName),
		QuotaBytes:            req.QuotaBytes,
		ObjectLimit:           req.ObjectLimit,
		VersioningEnabled:     req.VersioningEnabled,
		AccessLoggingEnabled:  req.AccessLoggingEnabled,
		AccessLogTargetBucket: emptyToNil(req.AccessLogTargetBucket),
		AccessLogPrefix:       emptyToNil(req.AccessLogPrefix),
		Status:                "active",
		CreatedBy:             &uid,
		CreatedAt:             now,
	}
	if err := h.store.Insert(r.Context(), b); err != nil {
		if errors.Is(err, ErrConflict) {
			httpx.Error(w, http.StatusConflict, "Bucket '"+req.BucketName+"' already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if h.s3 != nil {
		if err := h.s3.EnsureBucket(r.Context(), b.BucketName); err != nil {
			_ = h.store.Delete(r.Context(), b.ID)
			httpx.Error(w, http.StatusBadGateway, "storage error")
			return
		}
	}
	httpx.JSON(w, http.StatusCreated, toDetail(*b))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, _ *auth.User) {
	b, ok := h.load(w, r)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, toDetail(*b))
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request, _ *auth.User) {
	b, ok := h.load(w, r)
	if !ok {
		return
	}
	var req updateRequest
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.DisplayName != nil {
		b.DisplayName = emptyToNil(req.DisplayName)
	}
	if req.DisplayAliasOnly != nil {
		b.DisplayAliasOnly = *req.DisplayAliasOnly
	}
	if b.DisplayName == nil {
		b.DisplayAliasOnly = false
	}
	if req.QuotaBytes != nil {
		b.QuotaBytes = req.QuotaBytes
	}
	if req.ObjectLimit != nil {
		b.ObjectLimit = req.ObjectLimit
	}
	if req.VersioningEnabled != nil {
		b.VersioningEnabled = *req.VersioningEnabled
	}
	if req.AccessLoggingEnabled != nil {
		b.AccessLoggingEnabled = *req.AccessLoggingEnabled
	}
	if req.AccessLogTargetBucket != nil {
		b.AccessLogTargetBucket = emptyToNil(req.AccessLogTargetBucket)
	}
	if req.AccessLogPrefix != nil {
		b.AccessLogPrefix = emptyToNil(req.AccessLogPrefix)
	}
	if err := h.validateLogging(w, r, b.AccessLoggingEnabled, b.AccessLogTargetBucket, b.BucketName); err != nil {
		return
	}
	b.UpdatedAt = time.Now()
	if err := h.store.Update(r.Context(), b); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, toDetail(*b))
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request, _ *auth.User) {
	b, ok := h.load(w, r)
	if !ok {
		return
	}
	if h.s3 != nil {
		if err := h.s3.DeleteBucket(r.Context(), b.BucketName); err != nil {
			if errors.Is(err, storage.ErrNotEmpty) {
				httpx.Error(w, http.StatusConflict, "bucket.not_empty")
				return
			}
			if !errors.Is(err, storage.ErrNotFound) {
				httpx.Error(w, http.StatusBadGateway, "storage error")
				return
			}
		}
	}
	if err := h.store.Delete(r.Context(), b.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) load(w http.ResponseWriter, r *http.Request) (*Bucket, bool) {
	name := r.PathValue("bucket_name")
	b, err := h.store.GetByName(r.Context(), name)
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

func (h *Handler) validateLogging(w http.ResponseWriter, r *http.Request, enabled bool, target *string, source string) error {
	if !enabled {
		return nil
	}
	if target == nil || *target == "" {
		httpx.Error(w, http.StatusBadRequest, "access_log_target_bucket is required when access logging is enabled")
		return errLogging
	}
	if *target == source {
		httpx.Error(w, http.StatusBadRequest, "Access log target bucket must differ from the source bucket")
		return errLogging
	}
	other, err := h.store.GetByName(r.Context(), *target)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return errLogging
	}
	if other == nil || other.Status != "active" {
		httpx.Error(w, http.StatusBadRequest, "Access log target bucket '"+*target+"' not found")
		return errLogging
	}
	return nil
}

var errLogging = errors.New("logging")

func emptyToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func toSummary(b Bucket) summary {
	return summary{
		BucketName:        b.BucketName,
		DisplayName:       b.DisplayName,
		DisplayAliasOnly:  b.DisplayAliasOnly,
		QuotaBytes:        b.QuotaBytes,
		Status:            b.Status,
		VersioningEnabled: b.VersioningEnabled,
		CreatedAt:         b.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func toDetail(b Bucket) detail {
	return detail{
		ID:                    b.ID,
		BucketName:            b.BucketName,
		DisplayName:           b.DisplayName,
		DisplayAliasOnly:      b.DisplayAliasOnly,
		QuotaBytes:            b.QuotaBytes,
		ObjectLimit:           b.ObjectLimit,
		VersioningEnabled:     b.VersioningEnabled,
		AccessLoggingEnabled:  b.AccessLoggingEnabled,
		AccessLogTargetBucket: b.AccessLogTargetBucket,
		AccessLogPrefix:       b.AccessLogPrefix,
		Status:                b.Status,
		CreatedAt:             b.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:             b.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
