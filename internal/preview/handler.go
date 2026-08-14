package preview

import (
	"errors"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

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
	protect func(auth.UserHandler) http.HandlerFunc
	ac      *rbac.Checker
	log     *audit.Logger
}

func NewHandler(s3 *storage.Client, buckets *bucket.Store, protect func(auth.UserHandler) http.HandlerFunc, ac *rbac.Checker, log *audit.Logger) *Handler {
	return &Handler{s3: s3, buckets: buckets, protect: protect, ac: ac, log: log}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/preview/text", h.protect(h.text))
	mux.HandleFunc("GET /api/preview/image", h.protect(h.media("image", imageExt)))
	mux.HandleFunc("GET /api/preview/video", h.protect(h.media("video", videoExt)))
	mux.HandleFunc("GET /api/preview/audio", h.protect(h.media("audio", audioExt)))
	mux.HandleFunc("GET /api/preview/pdf", h.protect(h.media("pdf", pdfExt)))
}

func (h *Handler) text(w http.ResponseWriter, r *http.Request, user *auth.User) {
	b, key, meta, ok := h.resolve(w, r, user)
	if !ok {
		return
	}
	if !textOK(key, meta.ContentType) {
		httpx.Error(w, http.StatusBadRequest, "Object is not a supported text preview type")
		return
	}
	if meta.Size > maxTextBytes {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "Text preview limited to 524288 bytes")
		return
	}
	raw, ct, err := h.s3.GetObject(r.Context(), b.BucketName, key, maxTextBytes+1)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	truncated := len(raw) > maxTextBytes
	if truncated {
		raw = raw[:maxTextBytes]
	}
	content := string(raw)
	if !utf8.ValidString(content) {
		content = strings.ToValidUTF8(content, "\uFFFD")
	}
	contentType := meta.ContentType
	if contentType == nil {
		contentType = ct
	}
	h.log.Record(r, user, b.BucketName, key, "preview", "success", "")
	httpx.JSON(w, http.StatusOK, map[string]any{
		"content": content, "language": guessLang(key, contentType),
		"content_type": contentType, "size": meta.Size,
		"filename": filename(key), "truncated": truncated,
	})
}

func (h *Handler) media(label string, allowed map[string]bool) auth.UserHandler {
	return func(w http.ResponseWriter, r *http.Request, user *auth.User) {
		b, key, meta, ok := h.resolve(w, r, user)
		if !ok {
			return
		}
		if !allowExt(key, allowed) {
			httpx.Error(w, http.StatusBadRequest, "Object is not a supported "+label+" preview type")
			return
		}
		url, expires, err := h.s3.PresignPreview(r.Context(), b.BucketName, key)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "storage error")
			return
		}
		ct := meta.ContentType
		if ct == nil {
			if guessed := mime.TypeByExtension("." + fileExt(key)); guessed != "" {
				ct = &guessed
			}
		}
		h.log.Record(r, user, b.BucketName, key, "preview", "success", "")
		httpx.JSON(w, http.StatusOK, map[string]any{
			"preview_url": url, "expires_in": expires,
			"content_type": ct, "size": meta.Size, "filename": filename(key),
		})
	}
}

func (h *Handler) resolve(w http.ResponseWriter, r *http.Request, user *auth.User) (*bucket.Bucket, string, storage.ObjectMeta, bool) {
	var zero storage.ObjectMeta
	if h.s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return nil, "", zero, false
	}
	if h.buckets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return nil, "", zero, false
	}
	q := r.URL.Query()
	name, key := q.Get("bucket_name"), q.Get("object_key")
	if name == "" || !objects.ValidUserKey(key) {
		httpx.Error(w, http.StatusBadRequest, "Invalid object key")
		return nil, "", zero, false
	}
	b, err := h.buckets.GetVisible(r.Context(), auth.AccountID(user), name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, "", zero, false
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return nil, "", zero, false
	}
	if h.ac.Forbidden(w, r, user, b.BucketName, key, rbac.ActionRead) {
		return nil, "", zero, false
	}
	meta, err := h.s3.HeadObject(r.Context(), b.BucketName, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return nil, "", zero, false
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return nil, "", zero, false
	}
	return b, key, meta, true
}
