package objects

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

type trashItem struct {
	Key          string  `json:"key"`
	Size         int64   `json:"size"`
	ContentType  *string `json:"content_type"`
	LastModified *string `json:"last_modified"`
}

func (h *Handler) listTrash(w http.ResponseWriter, r *http.Request, user *auth.User) {
	b, ok := h.bucket(w, r, user)
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
	recs, err := h.store.ListTrash(r.Context(), b.ID, prefix, after, maxKeys+1)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	truncated := len(recs) > maxKeys
	if truncated {
		recs = recs[:maxKeys]
	}
	items := make([]trashItem, 0, len(recs))
	var token *string
	for _, rec := range recs {
		orig, ok := FromTrashKey(rec.Key)
		if !ok {
			continue
		}
		items = append(items, trashItem{
			Key: orig, Size: rec.Size, ContentType: rec.ContentType, LastModified: rec.LastModified,
		})
		k := rec.Key
		token = &k
	}
	if !truncated {
		token = nil
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"items":              items,
		"is_truncated":       truncated,
		"continuation_token": token,
	})
}

func (h *Handler) restoreTrash(w http.ResponseWriter, r *http.Request, user *auth.User) {
	h.runTrashOp(w, r, user, true)
}

func (h *Handler) purgeTrash(w http.ResponseWriter, r *http.Request, user *auth.User) {
	h.runTrashOp(w, r, user, false)
}

func (h *Handler) runTrashOp(w http.ResponseWriter, r *http.Request, user *auth.User, restore bool) {
	if !h.requireS3(w) {
		return
	}
	raw, ok := readKeys(w, r)
	if !ok {
		return
	}
	keys, err := uniqueOriginalKeys(raw)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	b, ok := h.bucket(w, r, user)
	if !ok {
		return
	}
	succeeded := make([]string, 0, len(keys))
	failed := make([]opFailure, 0)
	now := time.Now()
	for _, key := range keys {
		var opErr error
		if restore {
			opErr = h.restoreOne(r, user.ID, b.ID, b.BucketName, key, now)
		} else {
			opErr = h.purgeOne(r, b.ID, b.BucketName, key)
		}
		if opErr != nil {
			failed = append(failed, opFailure{Key: key, Error: trashErr(opErr)})
			continue
		}
		succeeded = append(succeeded, key)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"succeeded": succeeded, "failed": failed})
}

func (h *Handler) restoreOne(r *http.Request, userID, bucketID int64, bucketName, key string, now time.Time) error {
	trashKey := ToTrashKey(key)
	meta, err := h.s3.HeadObject(r.Context(), bucketName, trashKey)
	if err != nil {
		return err
	}
	if _, err := h.s3.HeadObject(r.Context(), bucketName, key); err == nil {
		return errLiveExists
	} else if !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	if _, err := h.s3.CopyObject(r.Context(), bucketName, key, bucketName, trashKey); err != nil {
		return err
	}
	if err := h.s3.DeleteObject(r.Context(), bucketName, trashKey); err != nil {
		return err
	}
	return h.store.MoveKey(r.Context(), bucketID, bucketName, trashKey, key, meta.Size, meta.ETag, meta.ContentType, userID, now)
}

func (h *Handler) purgeOne(r *http.Request, bucketID int64, bucketName, key string) error {
	trashKey := ToTrashKey(key)
	if _, err := h.s3.HeadObject(r.Context(), bucketName, trashKey); err != nil {
		return err
	}
	if err := h.s3.DeleteObject(r.Context(), bucketName, trashKey); err != nil {
		return err
	}
	return h.store.Delete(r.Context(), bucketID, trashKey)
}

var errLiveExists = errors.New("Original object already exists")

func trashErr(err error) string {
	if errors.Is(err, errLiveExists) {
		return errLiveExists.Error()
	}
	if errors.Is(err, storage.ErrNotFound) {
		return "Object not found in trash"
	}
	return "storage error"
}
