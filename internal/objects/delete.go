package objects

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
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
	b, ok := h.bucket(w, r, user)
	if !ok {
		return
	}
	// ponytail: sync even for large batches; T14 queues. permanent skips RBAC until T11.
	permanent := strings.EqualFold(r.URL.Query().Get("permanent"), "true")
	resolved, failed := h.expandDeleteKeys(r.Context(), b, keys)
	deleted := make([]string, 0, len(resolved))
	now := time.Now()
	for _, key := range resolved {
		if err := h.deleteOne(r.Context(), b, user.ID, key, permanent, now); err != nil {
			failed = append(failed, opFailure{Key: key, Error: deleteErr(err)})
			continue
		}
		deleted = append(deleted, key)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"deleted": deleted,
		"failed":  failed,
		"status":  "completed",
	})
}

func (h *Handler) expandDeleteKeys(ctx context.Context, b *bucket.Bucket, keys []string) ([]string, []opFailure) {
	seen := make(map[string]struct{}, len(keys))
	resolved := make([]string, 0, len(keys))
	var failed []opFailure
	for _, key := range keys {
		if !ValidUserKey(key) {
			failed = append(failed, opFailure{Key: key, Error: errInvalidKey.Error()})
			continue
		}
		if !IsDirectoryKey(key) {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			resolved = append(resolved, key)
			continue
		}
		children, err := h.store.ListLiveKeys(ctx, b.ID, key)
		if err != nil {
			failed = append(failed, opFailure{Key: key, Error: "database error"})
			continue
		}
		if len(children) == 0 {
			if _, err := h.s3.HeadObject(ctx, b.BucketName, key); err != nil {
				if errors.Is(err, storage.ErrNotFound) {
					failed = append(failed, opFailure{Key: key, Error: "Directory not found or empty"})
				} else {
					failed = append(failed, opFailure{Key: key, Error: "storage error"})
				}
				continue
			}
			children = []string{key}
		}
		for _, child := range children {
			if _, ok := seen[child]; ok {
				continue
			}
			seen[child] = struct{}{}
			resolved = append(resolved, child)
		}
	}
	if failed == nil {
		failed = []opFailure{}
	}
	return resolved, failed
}

func (h *Handler) deleteOne(ctx context.Context, b *bucket.Bucket, userID int64, key string, permanent bool, now time.Time) error {
	meta, err := h.s3.HeadObject(ctx, b.BucketName, key)
	if err != nil {
		return err
	}
	if permanent {
		if err := h.s3.DeleteObject(ctx, b.BucketName, key); err != nil {
			return err
		}
		return h.store.Delete(ctx, b.ID, key)
	}
	trashKey := ToTrashKey(key)
	if _, err := h.s3.CopyObject(ctx, b.BucketName, trashKey, b.BucketName, key); err != nil {
		return err
	}
	if err := h.s3.DeleteObject(ctx, b.BucketName, key); err != nil {
		return err
	}
	return h.store.MoveKey(ctx, b.ID, b.BucketName, key, trashKey, meta.Size, meta.ETag, meta.ContentType, userID, now)
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
