package project

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/creds"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type Handler struct {
	secret  string
	users   *auth.Store
	buckets *bucket.Store
	creds   *creds.Store
	queue   inventoryQueue
}

type inventoryQueue interface {
	EnqueueInventory(ctx context.Context, bucketName string) (string, error)
}

func NewHandler(secret string, users *auth.Store, buckets *bucket.Store, credsStore *creds.Store, q inventoryQueue) *Handler {
	return &Handler{secret: secret, users: users, buckets: buckets, creds: credsStore, queue: q}
}

type accountReq struct {
	Username           string  `json:"username"`
	PasswordHash       string  `json:"password_hash"`
	DisplayName        *string `json:"display_name"`
	Email              *string `json:"email"`
	Phone              *string `json:"phone"`
	Status             string  `json:"status"`
	MustChangePassword *bool   `json:"must_change_password"`
	QuotaBytes         *int64  `json:"quota_bytes"`
	ObjectLimit        *int64  `json:"object_limit"`
	DailyUploadBytes   *int64  `json:"daily_upload_bytes"`
}

type bucketsReq struct {
	Items []bucketItem `json:"items"`
}

type bucketItem struct {
	BucketName  string  `json:"bucket_name"`
	DisplayName *string `json:"display_name"`
}

func (h *Handler) upsert(w http.ResponseWriter, r *http.Request) {
	if !h.gate(w, r) {
		return
	}
	var req accountReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	username := r.PathValue("username")
	if username == "" {
		httpx.Error(w, http.StatusBadRequest, "username is required")
		return
	}
	status := req.Status
	if status == "" {
		status = "active"
	}
	if _, err := h.users.Upsert(r.Context(), auth.Upsert{
		Username:           username,
		PasswordHash:       req.PasswordHash,
		DisplayName:        req.DisplayName,
		Email:              req.Email,
		Phone:              req.Phone,
		Status:             status,
		MustChangePassword: req.MustChangePassword,
		QuotaBytes:         req.QuotaBytes,
		ObjectLimit:        req.ObjectLimit,
		DailyUploadBytes:   req.DailyUploadBytes,
	}); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	if !h.gate(w, r) {
		return
	}
	if err := h.users.DeleteByUsername(r.Context(), r.PathValue("username")); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) replaceBuckets(w http.ResponseWriter, r *http.Request) {
	if !h.gate(w, r) {
		return
	}
	var req bucketsReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	user, err := h.users.GetByUsername(r.Context(), r.PathValue("username"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if user == nil {
		httpx.Error(w, http.StatusNotFound, "Account not found")
		return
	}
	ids := make([]int64, 0, len(req.Items))
	for _, item := range req.Items {
		id, err := h.buckets.Ensure(r.Context(), item.BucketName, item.DisplayName)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		ids = append(ids, id)
	}
	if err := h.buckets.ReplaceOpsGrants(r.Context(), user.ID, ids); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) gate(w http.ResponseWriter, r *http.Request) bool {
	if h.secret == "" || h.users == nil || h.buckets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return false
	}
	if !bearerOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return false
	}
	return true
}

func bearerOK(header, secret string) bool {
	const p = "Bearer "
	if secret == "" || !strings.HasPrefix(header, p) {
		return false
	}
	got := strings.TrimSpace(header[len(p):])
	if len(got) != len(secret) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}
