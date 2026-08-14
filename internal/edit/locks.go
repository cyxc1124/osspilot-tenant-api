package edit

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
)

func (h *Handler) lockStatus(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireStore(w) {
		return
	}
	q := r.URL.Query()
	b, ok := h.visible(w, r, user, q.Get("bucket_name"), q.Get("object_key"))
	if !ok {
		return
	}
	lock, err := h.store.ActiveLock(r.Context(), b.BucketName, q.Get("object_key"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	readonly := lock != nil && lock.LockedBy != user.ID
	out := lockResponse(lock, readonly)
	out.LockToken = nil
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) lockAcquire(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireStore(w) {
		return
	}
	var req objectReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, ok := h.visible(w, r, user, req.BucketName, req.ObjectKey)
	if !ok {
		return
	}
	lock, readonly, err := h.acquire(r, b.BucketName, req.ObjectKey, user.ID, "")
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, lockResponse(lock, readonly))
}

func (h *Handler) lockRefresh(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireStore(w) {
		return
	}
	var req objectReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.LockToken == nil || *req.LockToken == "" {
		httpx.Error(w, http.StatusBadRequest, "lock_token is required")
		return
	}
	b, ok := h.visible(w, r, user, req.BucketName, req.ObjectKey)
	if !ok {
		return
	}
	lock, err := h.store.ActiveLock(r.Context(), b.BucketName, req.ObjectKey)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if lock == nil {
		httpx.Error(w, http.StatusNotFound, "No active lock found")
		return
	}
	if lock.LockedBy != user.ID {
		httpx.Error(w, http.StatusForbidden, "Not allowed to refresh this lock")
		return
	}
	if !tokenOK(lock.Token, *req.LockToken) {
		httpx.Error(w, http.StatusForbidden, "Invalid lock token")
		return
	}
	expires, err := h.store.refreshLock(r.Context(), lock.ID, nil, lockTTL)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	lock.ExpiresAt = expires
	httpx.JSON(w, http.StatusOK, lockResponse(lock, false))
}

func (h *Handler) lockRelease(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireStore(w) {
		return
	}
	var req objectReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, ok := h.visible(w, r, user, req.BucketName, req.ObjectKey)
	if !ok {
		return
	}
	okRel, err := h.releaseOwned(r, b.BucketName, req.ObjectKey, user.ID, req.LockToken)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if !okRel {
		httpx.Error(w, http.StatusForbidden, "Not allowed to unlock this file")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"unlocked": true})
}

func (h *Handler) forceUnlockInternal(w http.ResponseWriter, r *http.Request) {
	if h.secret == "" || h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return
	}
	if !internalBearerOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var req struct {
		BucketName string `json:"bucket_name"`
		ObjectKey  string `json:"object_key"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(req.BucketName) == "" || !objects.ValidUserKey(req.ObjectKey) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return
	}
	unlocked, err := h.store.ForceRelease(r.Context(), req.BucketName, req.ObjectKey)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"unlocked": unlocked})
}

func internalBearerOK(header, secret string) bool {
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
