package objects

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

type keysReq struct {
	Keys []string `json:"keys"`
}

type opFailure struct {
	Key   string `json:"key"`
	Error string `json:"error"`
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireS3(w) {
		return
	}
	keys, ok := readKeys(w, r)
	if !ok {
		return
	}
	for _, key := range keys {
		if key == "" || strings.HasPrefix(key, TrashPrefix) {
			httpx.Error(w, http.StatusBadRequest, errInvalidKey.Error())
			return
		}
	}
	b, ok := h.bucket(w, r, user, rbac.ActionDelete, "")
	if !ok {
		return
	}
	permanent := strings.EqualFold(r.URL.Query().Get("permanent"), "true")
	if shouldQueue(len(keys)) {
		if h.missingQueue(w, len(keys)) {
			return
		}
		h.queueDelete(w, r, user, b.BucketName, keys, permanent, len(keys))
		return
	}
	resolved, failed := ExpandDeleteKeys(r.Context(), h.s3, h.store, b, keys)
	if shouldQueue(len(resolved)) {
		if h.missingQueue(w, len(resolved)) {
			return
		}
		h.queueDelete(w, r, user, b.BucketName, keys, permanent, len(resolved))
		return
	}
	deleted := make([]string, 0, len(resolved))
	now := time.Now()
	for _, key := range resolved {
		if err := ApplyDelete(r.Context(), h.s3, h.store, b, user.ID, key, permanent, now); err != nil {
			failed = append(failed, opFailure{Key: key, Error: deleteErr(err)})
			h.log.Record(r, user, b.BucketName, key, "delete", "failure", deleteErr(err))
			continue
		}
		deleted = append(deleted, key)
		h.log.Record(r, user, b.BucketName, key, "delete", "success", "")
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"deleted": deleted,
		"failed":  failed,
		"status":  "completed",
	})
}

func deleteErr(err error) string {
	if errors.Is(err, storage.ErrNotFound) {
		return "Object not found"
	}
	return "storage error"
}

func readKeys(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var req keysReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return nil, false
	}
	if n := len(req.Keys); n < 1 || n > maxOpKeys {
		httpx.Error(w, http.StatusBadRequest, "keys must contain 1 to 1000 items")
		return nil, false
	}
	return req.Keys, true
}
