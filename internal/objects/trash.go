package objects

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

type trashItem struct {
	Key          string  `json:"key"`
	Size         int64   `json:"size"`
	ContentType  *string `json:"content_type"`
	LastModified *string `json:"last_modified"`
}

func (h *Handler) listTrash(w http.ResponseWriter, r *http.Request, user *auth.User) {
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
	s3 := h.client(r.Context())
	if s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return
	}
	items, token, truncated, s3Keys, err := ListTrashFromS3(r.Context(), s3, b.BucketName, prefix, after, maxKeys)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	if meta, err := h.store.MetaByKeys(r.Context(), b.ID, s3Keys); err == nil {
		now := time.Now()
		for i, trashKey := range s3Keys {
			if rec, ok := meta[trashKey]; ok && i < len(items) {
				items[i].ContentType = rec.ContentType
			}
			if i < len(items) {
				_ = h.store.UpsertSeen(r.Context(), b.ID, b.BucketName, trashKey, items[i].Size, nil, nil, now)
			}
		}
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
	action := rbac.ActionDelete
	name := "purge"
	if restore {
		action = rbac.ActionRestore
		name = "restore"
	}
	b, ok := h.visible(w, r, user)
	if !ok {
		return
	}
	if h.denyKeys(w, r, user, b.BucketName, action, keys) {
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
			h.log.Record(r, user, b.BucketName, key, name, "failure", trashErr(opErr))
			continue
		}
		succeeded = append(succeeded, key)
		h.log.Record(r, user, b.BucketName, key, name, "success", "")
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"succeeded": succeeded, "failed": failed})
}

func (h *Handler) restoreOne(r *http.Request, userID, bucketID int64, bucketName, key string, now time.Time) error {
	s3 := h.client(r.Context())
	trashKey := ToTrashKey(key)
	meta, err := s3.HeadObject(r.Context(), bucketName, trashKey)
	if err != nil {
		return err
	}
	if _, err := s3.HeadObject(r.Context(), bucketName, key); err == nil {
		return errLiveExists
	} else if !errors.Is(err, storage.ErrNotFound) {
		return err
	}
	if _, err := s3.CopyObject(r.Context(), bucketName, key, bucketName, trashKey); err != nil {
		return err
	}
	if err := s3.DeleteObject(r.Context(), bucketName, trashKey); err != nil {
		return err
	}
	return h.store.MoveKey(r.Context(), bucketID, bucketName, trashKey, key, meta.Size, meta.ETag, meta.ContentType, userID, now)
}

func (h *Handler) purgeOne(r *http.Request, bucketID int64, bucketName, key string) error {
	s3 := h.client(r.Context())
	trashKey := ToTrashKey(key)
	if _, err := s3.HeadObject(r.Context(), bucketName, trashKey); err != nil {
		return err
	}
	if err := s3.DeleteObject(r.Context(), bucketName, trashKey); err != nil {
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
