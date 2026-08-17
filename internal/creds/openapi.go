package creds

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/quota"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
	"github.com/cyxc1124/osspilot-tenant-api/internal/uploads"
)

type Principal struct {
	AccountID     int64
	ApplicationID int64
	ActingUserID  int64
	AccessKeyID   string
	AuthType      string
}

func (h *Handler) registerOpen(mux *http.ServeMux) {
	o := &openAPI{h: h, buckets: h.buckets, objects: h.objects, uploads: h.uploads}
	mux.HandleFunc("GET /api/v1/buckets", o.auth(o.listBuckets))
	mux.HandleFunc("GET /api/v1/buckets/{bucket_name}", o.auth(o.getBucket))
	mux.HandleFunc("GET /api/v1/buckets/{bucket_name}/objects", o.auth(o.listObjects))
	mux.HandleFunc("POST /api/v1/uploads/presign", o.auth(o.presignUpload))
	mux.HandleFunc("POST /api/v1/downloads/presign", o.auth(o.presignDownload))
	mux.HandleFunc("POST /api/v1/sts/credentials", o.auth(o.issueOpenSTS))
}

type openAPI struct {
	h       *Handler
	buckets *bucket.Store
	objects *objects.Store
	uploads *uploads.Store
}

type openHandler func(http.ResponseWriter, *http.Request, Principal)

func (o *openAPI) auth(next openHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if o.h.store == nil {
			httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
			return
		}
		p, err := o.h.authenticate(r)
		if err != nil {
			se, ok := err.(statusError)
			if !ok {
				httpx.Error(w, http.StatusUnauthorized, err.Error())
				return
			}
			httpx.Error(w, se.status, se.msg)
			return
		}
		r = o.h.users.BindS3(r, p.AccountID)
		next(w, r, p)
	}
}

type statusError struct {
	status int
	msg    string
}

func (e statusError) Error() string { return e.msg }

func (h *Handler) authenticate(r *http.Request) (Principal, error) {
	parsed, err := parseAuthorization(r.Header.Get("Authorization"))
	if err != nil {
		return Principal{}, statusError{http.StatusUnauthorized, err.Error()}
	}
	if parsed.Type == "sts_session" {
		if p, err := h.authSTS(r, parsed); err == nil {
			return p, nil
		}
	}
	key, err := h.store.GetKeyByAccessID(r.Context(), parsed.KeyID)
	if err != nil {
		return Principal{}, statusError{http.StatusInternalServerError, "database error"}
	}
	if key == nil || !auth.CheckPassword(parsed.Secret, key.SecretHash) {
		return Principal{}, statusError{http.StatusUnauthorized, "Invalid access key credentials"}
	}
	if key.Status != "active" {
		return Principal{}, statusError{http.StatusForbidden, "Access key is disabled"}
	}
	app, err := h.store.GetApp(r.Context(), key.AccountID, key.ApplicationID)
	if err != nil || app == nil {
		return Principal{}, statusError{http.StatusInternalServerError, "database error"}
	}
	if app.Status != "active" {
		return Principal{}, statusError{http.StatusForbidden, "Application is disabled"}
	}
	access, err := h.store.GetAccess(r.Context(), key.AccountID)
	if err != nil {
		return Principal{}, statusError{http.StatusInternalServerError, "database error"}
	}
	if access == nil || access.Status != "approved" {
		return Principal{}, statusError{http.StatusForbidden, "Tenant API access is not approved"}
	}
	if parsed.Type == "sts_session" {
		claims, err := parseSTS(h.secret, parsed.Session)
		if err != nil || claims.AccessKeyID != key.AccessKeyID || claims.AccountID != key.AccountID {
			return Principal{}, statusError{http.StatusUnauthorized, "Invalid STS session credentials"}
		}
	}
	_ = h.store.TouchKey(r.Context(), key.ID)
	return Principal{
		AccountID: key.AccountID, ApplicationID: key.ApplicationID, ActingUserID: app.CreatedBy,
		AccessKeyID: key.AccessKeyID, AuthType: parsed.Type,
	}, nil
}

func (o *openAPI) listBuckets(w http.ResponseWriter, r *http.Request, p Principal) {
	if o.buckets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	items, err := o.buckets.List(r.Context(), p.AccountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	ids := make([]int64, 0, len(items))
	for _, b := range items {
		ids = append(ids, b.ID)
	}
	usage, _ := o.buckets.UsageByID(r.Context(), ids)
	out := make([]map[string]any, 0, len(items))
	for _, b := range items {
		u := usage[b.ID]
		out = append(out, map[string]any{
			"bucket_name": b.BucketName, "display_name": b.DisplayName, "display_alias_only": b.DisplayAliasOnly,
			"quota_bytes": b.QuotaBytes, "used_bytes": u.UsedBytes, "object_count": u.ObjectCount, "status": b.Status,
			"versioning_enabled": b.VersioningEnabled, "created_at": b.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (o *openAPI) getBucket(w http.ResponseWriter, r *http.Request, p Principal) {
	if o.buckets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	b, err := o.buckets.GetVisible(r.Context(), p.AccountID, r.PathValue("bucket_name"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return
	}
	usage, _ := o.buckets.UsageByID(r.Context(), []int64{b.ID})
	u := usage[b.ID]
	httpx.JSON(w, http.StatusOK, map[string]any{
		"id": b.ID, "bucket_name": b.BucketName, "display_name": b.DisplayName, "display_alias_only": b.DisplayAliasOnly,
		"quota_bytes": b.QuotaBytes, "object_limit": b.ObjectLimit, "used_bytes": u.UsedBytes, "object_count": u.ObjectCount,
		"versioning_enabled": b.VersioningEnabled, "access_logging_enabled": b.AccessLoggingEnabled,
		"access_log_target_bucket": b.AccessLogTargetBucket, "access_log_prefix": b.AccessLogPrefix,
		"status": b.Status, "created_at": b.CreatedAt.UTC().Format(time.RFC3339), "updated_at": b.UpdatedAt.UTC().Format(time.RFC3339),
	})
}

func (o *openAPI) listObjects(w http.ResponseWriter, r *http.Request, p Principal) {
	if o.buckets == nil || o.objects == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	b, err := o.buckets.GetVisible(r.Context(), p.AccountID, r.PathValue("bucket_name"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return
	}
	q := r.URL.Query()
	maxKeys := 1000
	if raw := q.Get("max_keys"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "max_keys must be an integer")
			return
		}
		maxKeys = n
	}
	s3 := o.h.client(r)
	if s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return
	}
	if maxKeys < 1 {
		maxKeys = 1
	}
	if maxKeys > 1000 {
		maxKeys = 1000
	}
	items, prefixes, token, truncated, err := objects.ListFromS3(r.Context(), s3, b.BucketName, q.Get("prefix"), q.Get("continuation_token"), maxKeys)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	keys := make([]string, 0, len(items))
	now := time.Now()
	for _, rec := range items {
		keys = append(keys, rec.Key)
		_ = o.objects.UpsertSeen(r.Context(), b.ID, b.BucketName, rec.Key, rec.Size, rec.ETag, nil, now)
	}
	meta, _ := o.objects.MetaByKeys(r.Context(), b.ID, keys)
	out := make([]map[string]any, 0, len(items))
	for _, rec := range items {
		if m, ok := meta[rec.Key]; ok {
			rec.ContentType = m.ContentType
			rec.UploadedBy = m.UploadedBy
		}
		out = append(out, map[string]any{
			"key": rec.Key, "size": rec.Size, "content_type": rec.ContentType,
			"last_modified": rec.LastModified, "etag": rec.ETag, "uploaded_by": rec.UploadedBy,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"items": out, "prefixes": prefixes, "is_truncated": truncated, "continuation_token": token,
	})
}

func (o *openAPI) presignUpload(w http.ResponseWriter, r *http.Request, p Principal) {
	s3 := o.h.client(r)
	if s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return
	}
	if o.buckets == nil || o.uploads == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return
	}
	var req struct {
		BucketName  string  `json:"bucket_name"`
		ObjectKey   string  `json:"object_key"`
		Size        int64   `json:"size"`
		ContentType *string `json:"content_type"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !objects.ValidUserKey(req.ObjectKey) || req.Size <= 0 {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return
	}
	b, err := o.buckets.GetVisible(r.Context(), p.AccountID, req.BucketName)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return
	}
	if err := o.h.quota.Upload(r.Context(), p.AccountID, b, req.ObjectKey, req.Size); err != nil {
		quota.Reject(w, err)
		return
	}
	ct := ""
	if req.ContentType != nil {
		ct = *req.ContentType
	}
	url, expires, err := s3.PresignPut(r.Context(), b.BucketName, req.ObjectKey, ct)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	task := &uploads.Task{UserID: p.ActingUserID, BucketName: b.BucketName, ObjectKey: req.ObjectKey, UploadType: uploads.TypeSimple, Size: &req.Size, ContentType: req.ContentType}
	if err := o.uploads.Insert(r.Context(), task, time.Now().Add(time.Duration(expires)*time.Second)); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	headers := map[string]string{}
	if ct != "" {
		headers["Content-Type"] = ct
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"task_id": task.ID, "upload_url": url, "headers": headers, "expires_in": expires})
}

func (o *openAPI) presignDownload(w http.ResponseWriter, r *http.Request, p Principal) {
	s3 := o.h.client(r)
	if s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return
	}
	if o.buckets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
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
	if !objects.ValidUserKey(req.ObjectKey) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return
	}
	b, err := o.buckets.GetVisible(r.Context(), p.AccountID, req.BucketName)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return
	}
	if _, err := s3.HeadObject(r.Context(), b.BucketName, req.ObjectKey); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	url, expires, err := s3.PresignGet(r.Context(), b.BucketName, req.ObjectKey)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	o.h.log.RecordMeta(r.Context(), p.ActingUserID, p.AccountID, b.BucketName, req.ObjectKey, "download", "success", "", "", "")
	httpx.JSON(w, http.StatusOK, map[string]any{"download_url": url, "expires_in": expires})
}

func (o *openAPI) issueOpenSTS(w http.ResponseWriter, r *http.Request, p Principal) {
	if p.AuthType != "access_key" {
		httpx.Error(w, http.StatusBadRequest, "STS issuance requires OSSAccessKey authentication")
		return
	}
	var req stsReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.AccessKeyID != p.AccessKeyID {
		httpx.Error(w, http.StatusBadRequest, "access_key_id must match Authorization")
		return
	}
	parsed, err := parseAuthorization(r.Header.Get("Authorization"))
	if err != nil {
		httpx.Error(w, http.StatusUnauthorized, err.Error())
		return
	}
	dur := req.DurationSeconds
	if dur == 0 {
		dur = defaultSTS
	}
	dur = clampSTS(dur)
	key, err := o.h.store.GetKeyByAccessID(r.Context(), p.AccessKeyID)
	if err != nil || key == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := o.h.issueToken(r, key, parsed.Secret, dur)
	if out == nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	httpx.JSON(w, http.StatusOK, out)
}
