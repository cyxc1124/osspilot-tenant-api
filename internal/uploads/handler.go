package uploads

import (
	"errors"
	"net/http"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/audit"
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
	objects *objects.Store
	tasks   *Store
	protect func(auth.UserHandler) http.HandlerFunc
	ac      *rbac.Checker
	log     *audit.Logger
}

func NewHandler(s3 *storage.Client, buckets *bucket.Store, objects *objects.Store, tasks *Store, protect func(auth.UserHandler) http.HandlerFunc, ac *rbac.Checker, log *audit.Logger) *Handler {
	return &Handler{s3: s3, buckets: buckets, objects: objects, tasks: tasks, protect: protect, ac: ac, log: log}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/uploads/presign", h.protect(h.presign))
	mux.HandleFunc("POST /api/uploads/complete", h.protect(h.complete))
	mux.HandleFunc("POST /api/uploads/multipart/init", h.protect(h.mpInit))
	mux.HandleFunc("POST /api/uploads/multipart/parts", h.protect(h.mpParts))
	mux.HandleFunc("POST /api/uploads/multipart/complete", h.protect(h.mpComplete))
	mux.HandleFunc("POST /api/uploads/multipart/abort", h.protect(h.mpAbort))
}

type presignReq struct {
	BucketName  string  `json:"bucket_name"`
	ObjectKey   string  `json:"object_key"`
	Size        int64   `json:"size"`
	ContentType *string `json:"content_type"`
}

type completeReq struct {
	BucketName string `json:"bucket_name"`
	ObjectKey  string `json:"object_key"`
	TaskID     int64  `json:"task_id"`
}

type mpInitReq struct {
	BucketName  string  `json:"bucket_name"`
	ObjectKey   string  `json:"object_key"`
	Size        int64   `json:"size"`
	ContentType *string `json:"content_type"`
}

type mpPartsReq struct {
	BucketName  string  `json:"bucket_name"`
	ObjectKey   string  `json:"object_key"`
	UploadID    string  `json:"upload_id"`
	TaskID      int64   `json:"task_id"`
	PartNumbers []int32 `json:"part_numbers"`
}

type mpCompleteReq struct {
	BucketName string `json:"bucket_name"`
	ObjectKey  string `json:"object_key"`
	UploadID   string `json:"upload_id"`
	TaskID     int64  `json:"task_id"`
	Parts      []struct {
		PartNumber int32  `json:"part_number"`
		ETag       string `json:"etag"`
	} `json:"parts"`
}

type mpAbortReq struct {
	BucketName string `json:"bucket_name"`
	ObjectKey  string `json:"object_key"`
	UploadID   string `json:"upload_id"`
	TaskID     int64  `json:"task_id"`
}

func (h *Handler) presign(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) {
		return
	}
	var req presignReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, ct, ok := h.prepare(w, r, user, req.BucketName, req.ObjectKey, req.Size, req.ContentType)
	if !ok {
		return
	}
	url, expires, err := h.s3.PresignPut(r.Context(), b.BucketName, req.ObjectKey, ct)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	task := &Task{UserID: user.ID, BucketName: b.BucketName, ObjectKey: req.ObjectKey, UploadType: TypeSimple, Size: &req.Size, ContentType: req.ContentType}
	if err := h.tasks.Insert(r.Context(), task, time.Now().Add(time.Duration(expires)*time.Second)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	headers := map[string]string{}
	if ct != "" {
		headers["Content-Type"] = ct
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"task_id": task.ID, "upload_url": url, "headers": headers, "expires_in": expires,
	})
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) {
		return
	}
	var req completeReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	h.finalize(w, r, user, req.BucketName, req.ObjectKey, req.TaskID, TypeSimple, nil)
}

func (h *Handler) mpInit(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) {
		return
	}
	var req mpInitReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, ct, ok := h.prepare(w, r, user, req.BucketName, req.ObjectKey, req.Size, req.ContentType)
	if !ok {
		return
	}
	uploadID, err := h.s3.CreateMultipart(r.Context(), b.BucketName, req.ObjectKey, ct)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	task := &Task{UserID: user.ID, BucketName: b.BucketName, ObjectKey: req.ObjectKey, UploadType: TypeMultipart, UploadID: &uploadID, Size: &req.Size, ContentType: req.ContentType}
	if err := h.tasks.Insert(r.Context(), task, time.Now().Add(time.Duration(h.s3.UploadTTLSeconds())*time.Second)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"task_id": task.ID, "upload_id": uploadID})
}

func (h *Handler) mpParts(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) {
		return
	}
	var req mpPartsReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if _, ok := h.bucketOK(w, r, user, req.BucketName, req.ObjectKey); !ok {
		return
	}
	task, ok := h.pending(w, r, user.ID, req.TaskID, req.BucketName, req.ObjectKey, TypeMultipart)
	if !ok {
		return
	}
	if task.UploadID == nil || *task.UploadID != req.UploadID {
		httpx.Error(w, http.StatusBadRequest, "Upload ID does not match task")
		return
	}
	if len(req.PartNumbers) == 0 {
		httpx.Error(w, http.StatusBadRequest, "part_numbers required")
		return
	}
	parts := make([]map[string]any, 0, len(req.PartNumbers))
	for _, n := range req.PartNumbers {
		if n < 1 {
			httpx.Error(w, http.StatusBadRequest, "Invalid part number")
			return
		}
		url, err := h.s3.PresignUploadPart(r.Context(), req.BucketName, req.ObjectKey, req.UploadID, n)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "storage error")
			return
		}
		parts = append(parts, map[string]any{"part_number": n, "url": url})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"parts": parts, "expires_in": h.s3.UploadTTLSeconds()})
}

func (h *Handler) mpComplete(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) {
		return
	}
	var req mpCompleteReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	completed := make([]storage.CompletedPart, 0, len(req.Parts))
	for _, p := range req.Parts {
		completed = append(completed, storage.CompletedPart{PartNumber: p.PartNumber, ETag: p.ETag})
	}
	h.finalize(w, r, user, req.BucketName, req.ObjectKey, req.TaskID, TypeMultipart, func() error {
		return h.s3.CompleteMultipart(r.Context(), req.BucketName, req.ObjectKey, req.UploadID, completed)
	})
}

func (h *Handler) mpAbort(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) {
		return
	}
	var req mpAbortReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	task, ok := h.pending(w, r, user.ID, req.TaskID, req.BucketName, req.ObjectKey, TypeMultipart)
	if !ok {
		return
	}
	_ = h.s3.AbortMultipart(r.Context(), req.BucketName, req.ObjectKey, req.UploadID)
	_ = h.tasks.Finish(r.Context(), task.ID, StatusAbort, time.Now())
	httpx.JSON(w, http.StatusOK, map[string]any{"task_id": task.ID, "status": StatusAbort})
}

func (h *Handler) finalize(w http.ResponseWriter, r *http.Request, user *auth.User, bucketName, key string, taskID int64, uploadType string, beforeHead func() error) {
	if !objects.ValidUserKey(key) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return
	}
	b, ok := h.see(w, r, user, bucketName, key)
	if !ok {
		return
	}
	task, ok := h.pending(w, r, user.ID, taskID, bucketName, key, uploadType)
	if !ok {
		return
	}
	if beforeHead != nil {
		if err := beforeHead(); err != nil {
			httpx.Error(w, http.StatusBadGateway, "storage error")
			return
		}
	}
	meta, err := h.s3.HeadObject(r.Context(), b.BucketName, key)
	if errors.Is(err, storage.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "Uploaded object not found in storage")
		return
	}
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	if task.Size != nil && meta.Size != *task.Size {
		httpx.Error(w, http.StatusBadRequest, "Uploaded object size does not match expected size")
		return
	}
	now := time.Now()
	if err := h.objects.Upsert(r.Context(), b.ID, b.BucketName, key, meta.Size, meta.ETag, meta.ContentType, user.ID, now); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	_ = h.tasks.Finish(r.Context(), task.ID, StatusDone, now)
	h.log.Record(r, user, b.BucketName, key, "upload", "success", "")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"bucket_name": b.BucketName, "object_key": key, "size": meta.Size,
		"content_type": meta.ContentType, "etag": meta.ETag,
	})
}

func (h *Handler) prepare(w http.ResponseWriter, r *http.Request, user *auth.User, bucketName, key string, size int64, contentType *string) (*bucket.Bucket, string, bool) {
	if !objects.ValidUserKey(key) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return nil, "", false
	}
	if size <= 0 {
		httpx.Error(w, http.StatusBadRequest, "size must be greater than 0")
		return nil, "", false
	}
	b, ok := h.see(w, r, user, bucketName, key)
	if !ok {
		return nil, "", false
	}
	ct := ""
	if contentType != nil {
		ct = *contentType
	}
	return b, ct, true
}

func (h *Handler) bucketOK(w http.ResponseWriter, r *http.Request, user *auth.User, name, key string) (*bucket.Bucket, bool) {
	return h.see(w, r, user, name, key)
}

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
	if h.ac.Forbidden(w, r, user, b.BucketName, key, rbac.ActionWrite) {
		return nil, false
	}
	return b, true
}

func (h *Handler) pending(w http.ResponseWriter, r *http.Request, userID, taskID int64, bucket, key, uploadType string) (*Task, bool) {
	task, err := h.tasks.GetPending(r.Context(), taskID, userID, bucket, key, uploadType)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	if task == nil {
		httpx.Error(w, http.StatusNotFound, "Upload task not found")
		return nil, false
	}
	return task, true
}

func (h *Handler) ready(w http.ResponseWriter) bool {
	if h.s3 == nil || h.tasks == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return false
	}
	return true
}
