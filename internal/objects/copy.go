package objects

import (
	"errors"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

type keyPair struct {
	SourceKey      string  `json:"source_key"`
	DestKey        string  `json:"dest_key"`
	DestBucketName *string `json:"dest_bucket_name"`
}

type copyReq struct {
	Items []keyPair `json:"items"`
}

type renameReq struct {
	Key     string `json:"key"`
	NewName string `json:"new_name"`
}

func (h *Handler) RegisterCopy(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/buckets/{bucket_name}/objects/copy", h.protect(h.copy))
	mux.HandleFunc("POST /api/buckets/{bucket_name}/objects/move", h.protect(h.move))
	mux.HandleFunc("POST /api/buckets/{bucket_name}/objects/rename", h.protect(h.rename))
}

func (h *Handler) copy(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireS3(w) {
		return
	}
	items, ok := readPairs(w, r)
	if !ok {
		return
	}
	src, ok := h.bucket(w, r, user, rbac.ActionRead, "")
	if !ok {
		return
	}
	copied := make([]map[string]any, 0, len(items))
	failed := make([]map[string]string, 0)
	now := time.Now()
	for _, it := range items {
		destName := src.BucketName
		if it.DestBucketName != nil && strings.TrimSpace(*it.DestBucketName) != "" {
			destName = strings.TrimSpace(*it.DestBucketName)
		}
		destBucket, errMsg := h.visibleBucket(r, user, destName)
		if errMsg != "" {
			failed = append(failed, map[string]string{"source_key": it.SourceKey, "error": errMsg})
			continue
		}
		if h.ac.Forbidden(w, r, user, destBucket.BucketName, it.DestKey, rbac.ActionWrite) {
			return
		}
		if !ValidUserKey(it.SourceKey) || !ValidUserKey(it.DestKey) {
			failed = append(failed, map[string]string{"source_key": it.SourceKey, "error": "Invalid object key"})
			continue
		}
		meta, err := h.s3.HeadObject(r.Context(), src.BucketName, it.SourceKey)
		if err != nil {
			failed = append(failed, map[string]string{"source_key": it.SourceKey, "error": copyErr(err)})
			h.log.Record(r, user, src.BucketName, it.SourceKey, "copy", "failure", copyErr(err))
			continue
		}
		etag, err := h.s3.CopyObject(r.Context(), destBucket.BucketName, it.DestKey, src.BucketName, it.SourceKey)
		if err != nil {
			failed = append(failed, map[string]string{"source_key": it.SourceKey, "error": copyErr(err)})
			h.log.Record(r, user, destBucket.BucketName, it.DestKey, "copy", "failure", copyErr(err))
			continue
		}
		if err := h.store.Upsert(r.Context(), destBucket.ID, destBucket.BucketName, it.DestKey, meta.Size, etag, meta.ContentType, user.ID, now); err != nil {
			failed = append(failed, map[string]string{"source_key": it.SourceKey, "error": "database error"})
			continue
		}
		row := map[string]any{"source_key": it.SourceKey, "dest_key": it.DestKey, "dest_bucket_name": nil}
		if destName != src.BucketName {
			row["dest_bucket_name"] = destName
		}
		copied = append(copied, row)
		h.log.Record(r, user, destBucket.BucketName, it.DestKey, "copy", "success", "")
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"copied": copied, "failed": failed, "status": "completed"})
}

func (h *Handler) move(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireS3(w) {
		return
	}
	items, ok := readPairs(w, r)
	if !ok {
		return
	}
	b, ok := h.bucket(w, r, user, rbac.ActionWrite, "")
	if !ok {
		return
	}
	moved := make([]map[string]string, 0, len(items))
	failed := make([]map[string]string, 0)
	now := time.Now()
	for _, it := range items {
		if it.DestBucketName != nil && strings.TrimSpace(*it.DestBucketName) != "" && strings.TrimSpace(*it.DestBucketName) != b.BucketName {
			failed = append(failed, map[string]string{"source_key": it.SourceKey, "error": "Cross-bucket move is not supported"})
			continue
		}
		if !ValidUserKey(it.SourceKey) || !ValidUserKey(it.DestKey) {
			failed = append(failed, map[string]string{"source_key": it.SourceKey, "error": "Invalid object key"})
			continue
		}
		if h.ac.Forbidden(w, r, user, b.BucketName, it.SourceKey, rbac.ActionDelete) {
			return
		}
		if h.ac.Forbidden(w, r, user, b.BucketName, it.DestKey, rbac.ActionWrite) {
			return
		}
		if err := h.moveOne(r, user, b, it.SourceKey, it.DestKey, now); err != nil {
			failed = append(failed, map[string]string{"source_key": it.SourceKey, "error": copyErr(err)})
			h.log.Record(r, user, b.BucketName, it.SourceKey, "move", "failure", copyErr(err))
			continue
		}
		moved = append(moved, map[string]string{"source_key": it.SourceKey, "dest_key": it.DestKey})
		h.log.Record(r, user, b.BucketName, it.DestKey, "move", "success", "")
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"moved": moved, "failed": failed, "status": "completed"})
}

func (h *Handler) rename(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireS3(w) {
		return
	}
	var req renameReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	dest, err := renameDest(req.Key, req.NewName)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	b, ok := h.bucket(w, r, user, rbac.ActionWrite, req.Key)
	if !ok {
		return
	}
	if h.ac.Forbidden(w, r, user, b.BucketName, dest, rbac.ActionWrite) {
		return
	}
	if h.ac.Forbidden(w, r, user, b.BucketName, req.Key, rbac.ActionDelete) {
		return
	}
	if err := h.moveOne(r, user, b, req.Key, dest, time.Now()); err != nil {
		msg := copyErr(err)
		if msg == "Object not found" {
			httpx.Error(w, http.StatusNotFound, msg)
			return
		}
		httpx.Error(w, http.StatusBadRequest, msg)
		return
	}
	h.log.Record(r, user, b.BucketName, dest, "rename", "success", "")
	httpx.JSON(w, http.StatusOK, map[string]any{"source_key": req.Key, "dest_key": dest})
}

func (h *Handler) moveOne(r *http.Request, user *auth.User, b *bucket.Bucket, src, dest string, now time.Time) error {
	meta, err := h.s3.HeadObject(r.Context(), b.BucketName, src)
	if err != nil {
		return err
	}
	etag, err := h.s3.CopyObject(r.Context(), b.BucketName, dest, b.BucketName, src)
	if err != nil {
		return err
	}
	if err := h.s3.DeleteObject(r.Context(), b.BucketName, src); err != nil {
		return err
	}
	return h.store.MoveKey(r.Context(), b.ID, b.BucketName, src, dest, meta.Size, etag, meta.ContentType, user.ID, now)
}

func (h *Handler) visibleBucket(r *http.Request, user *auth.User, name string) (*bucket.Bucket, string) {
	b, err := h.buckets.GetVisible(r.Context(), auth.AccountID(user), name)
	if err != nil {
		return nil, "database error"
	}
	if b == nil {
		return nil, "Bucket not found"
	}
	return b, ""
}

func readPairs(w http.ResponseWriter, r *http.Request) ([]keyPair, bool) {
	var req copyReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return nil, false
	}
	if len(req.Items) == 0 || len(req.Items) > 1000 {
		httpx.Error(w, http.StatusBadRequest, "items must contain 1-1000 entries")
		return nil, false
	}
	return req.Items, true
}

func renameDest(key, newName string) (string, error) {
	name := strings.TrimSpace(newName)
	if name == "" || strings.Contains(strings.Trim(name, "/"), "/") {
		return "", errors.New("New name must not contain /")
	}
	if strings.Contains(key, "/") {
		return path.Dir(key) + "/" + name, nil
	}
	return name, nil
}

func copyErr(err error) string {
	if errors.Is(err, storage.ErrNotFound) {
		return "Object not found"
	}
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}
