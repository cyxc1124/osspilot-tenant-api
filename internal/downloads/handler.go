package downloads

import (
	"errors"
	"net/http"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

type Handler struct {
	s3      *storage.Client
	buckets *bucket.Store
	protect func(auth.UserHandler) http.HandlerFunc
	ac      *rbac.Checker
}

func NewHandler(s3 *storage.Client, buckets *bucket.Store, protect func(auth.UserHandler) http.HandlerFunc, ac *rbac.Checker) *Handler {
	return &Handler{s3: s3, buckets: buckets, protect: protect, ac: ac}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/downloads/presign", h.protect(h.presign))
	mux.HandleFunc("POST /api/downloads/batch", h.protect(h.batch))
}

type presignReq struct {
	BucketName string `json:"bucket_name"`
	ObjectKey  string `json:"object_key"`
}

type batchReq struct {
	BucketName string   `json:"bucket_name"`
	Keys       []string `json:"keys"`
}

func (h *Handler) presign(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if h.s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return
	}
	var req presignReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	url, expires, err := h.one(w, r, user, req.BucketName, req.ObjectKey)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"download_url": url, "expires_in": expires})
}

func (h *Handler) batch(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if h.s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return
	}
	var req batchReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.Keys) == 0 || len(req.Keys) > 1000 {
		httpx.Error(w, http.StatusBadRequest, "keys must contain 1-1000 items")
		return
	}
	b, ok := h.see(w, r, user, req.BucketName, "")
	if !ok {
		return
	}
	seen := map[string]bool{}
	items := make([]map[string]any, 0, len(req.Keys))
	expires := 0
	for _, key := range req.Keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		url, exp, err := h.presignKey(r, user, b.BucketName, key)
		if err != nil {
			items = append(items, map[string]any{"key": key, "download_url": nil, "error": err.Error()})
			continue
		}
		expires = exp
		items = append(items, map[string]any{"key": key, "download_url": url, "error": nil})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items, "expires_in": expires})
}

func (h *Handler) one(w http.ResponseWriter, r *http.Request, user *auth.User, bucketName, key string) (string, int, error) {
	if !objects.ValidUserKey(key) {
		return "", 0, badRequest("Invalid object key")
	}
	b, ok := h.see(w, r, user, bucketName, key)
	if !ok {
		return "", 0, errSilent
	}
	if _, err := h.s3.HeadObject(r.Context(), b.BucketName, key); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return "", 0, notFound("Object not found")
		}
		return "", 0, errors.New("storage error")
	}
	url, expires, err := h.s3.PresignGet(r.Context(), b.BucketName, key)
	if err != nil {
		return "", 0, errors.New("storage error")
	}
	return url, expires, nil
}

func (h *Handler) presignKey(r *http.Request, user *auth.User, bucketName, key string) (string, int, error) {
	if !objects.ValidUserKey(key) {
		return "", 0, badRequest("Invalid object key")
	}
	if h.ac != nil {
		ok, err := h.ac.Allow(r.Context(), user, bucketName, key, rbac.ActionRead)
		if err != nil {
			return "", 0, errors.New("database error")
		}
		if !ok {
			return "", 0, statusError{http.StatusForbidden, "Permission denied"}
		}
	}
	if _, err := h.s3.HeadObject(r.Context(), bucketName, key); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return "", 0, notFound("Object not found")
		}
		return "", 0, errors.New("storage error")
	}
	url, expires, err := h.s3.PresignGet(r.Context(), bucketName, key)
	if err != nil {
		return "", 0, errors.New("storage error")
	}
	return url, expires, nil
}

type statusError struct {
	status int
	msg    string
}

func (e statusError) Error() string { return e.msg }

var errSilent = errors.New("written")

func badRequest(msg string) error { return statusError{http.StatusBadRequest, msg} }
func notFound(msg string) error   { return statusError{http.StatusNotFound, msg} }

func (h *Handler) see(w http.ResponseWriter, r *http.Request, user *auth.User, name, key string) (*bucket.Bucket, bool) {
	b, err := h.buckets.GetVisible(r.Context(), auth.AccountID(user), name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return nil, false
	}
	if h.ac.Forbidden(w, r, user, b.BucketName, key, rbac.ActionRead) {
		return nil, false
	}
	return b, true
}

func (h *Handler) writeErr(w http.ResponseWriter, err error) {
	if errors.Is(err, errSilent) {
		return
	}
	var se statusError
	if errors.As(err, &se) {
		httpx.Error(w, se.status, se.msg)
		return
	}
	httpx.Error(w, http.StatusBadGateway, "storage error")
}
