package edit

import (
	"bytes"
	"errors"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
	"github.com/cyxc1124/osspilot-tenant-api/internal/versions"
)

type textOpenReq struct {
	BucketName string `json:"bucket_name"`
	ObjectKey  string `json:"object_key"`
	TenantID   *int64 `json:"tenant_id"`
}

type textSaveReq struct {
	Content string  `json:"content"`
	Remark  *string `json:"remark"`
}

type textCloseReq struct {
	SessionID string `json:"session_id"`
}

func (h *Handler) textOpen(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireStore(w) || !h.requireS3(w) {
		return
	}
	var req textOpenReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, ok := h.visible(w, r, user, req.BucketName, req.ObjectKey)
	if !ok {
		return
	}
	meta, err := h.s3.HeadObject(r.Context(), b.BucketName, req.ObjectKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	ct := ""
	if meta.ContentType != nil {
		ct = *meta.ContentType
	}
	if !textEditable(req.ObjectKey, ct) {
		httpx.Error(w, http.StatusBadRequest, "File type is not supported for text editing")
		return
	}
	if meta.Size > maxTextBytes {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "File too large for text editing")
		return
	}
	_ = h.store.expireSessions(r.Context(), b.BucketName, req.ObjectKey)
	raw, _, err := h.s3.GetObject(r.Context(), b.BucketName, req.ObjectKey, maxTextBytes+1)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	content := decodeText(raw)
	sid, err := newSessionID()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	expires := time.Now().UTC().Add(sessionTTL)
	existing, err := h.store.ActiveLock(r.Context(), b.BucketName, req.ObjectKey)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	readonly := existing != nil && existing.LockedBy != user.ID
	sess := &Session{
		ID:         sid,
		Bucket:     b.BucketName,
		Key:        req.ObjectKey,
		UserID:     user.ID,
		EditorType: editorMonaco,
		Mode:       modeEdit,
		ExpiresAt:  expires,
	}
	lockToken := ""
	locked := true
	if readonly {
		sess.Mode = modeView
		locked = false
	} else {
		tok, err := newToken()
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "token error")
			return
		}
		lockToken = tok
		sess.CallbackToken = tok
		if _, _, err := h.acquire(r, b.BucketName, req.ObjectKey, user.ID, tok); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
	}
	if err := h.store.InsertSession(r.Context(), sess); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"session_id":   sid,
		"lock_token":   lockToken,
		"bucket_name":  b.BucketName,
		"object_key":   req.ObjectKey,
		"content":      content,
		"language":     guessLanguage(req.ObjectKey, ct),
		"content_type": meta.ContentType,
		"size":         meta.Size,
		"locked":       locked,
		"readonly":     readonly,
		"expires_at":   formatTime(expires),
	})
}

func (h *Handler) textSave(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireStore(w) || !h.requireS3(w) {
		return
	}
	sess := h.loadTextSession(w, r, user)
	if sess == nil {
		return
	}
	if sess.Mode == modeView {
		httpx.Error(w, http.StatusForbidden, "Read-only preview cannot be saved")
		return
	}
	if !h.requireSessionLock(w, r, sess) {
		return
	}
	var req textSaveReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	body := []byte(req.Content)
	if len(body) > maxTextBytes {
		httpx.Error(w, http.StatusRequestEntityTooLarge, "File too large for text editing")
		return
	}
	n, err := versions.Archive(r.Context(), h.s3, h.versions, user.ID, sess.Bucket, sess.Key, versions.SourceTextEdit, req.Remark)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	ct := mime.TypeByExtension(path.Ext(sess.Key))
	if ct == "" {
		ct = "text/plain"
	}
	etag, err := h.s3.PutObject(r.Context(), sess.Bucket, sess.Key, bytes.NewReader(body), ct)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"saved":      true,
		"version_no": n,
		"etag":       etag,
		"saved_at":   formatTime(time.Now()),
	})
}

func (h *Handler) textUnlock(w http.ResponseWriter, r *http.Request, user *auth.User) {
	h.lockRelease(w, r, user)
}

func (h *Handler) textClose(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireStore(w) {
		return
	}
	var req textCloseReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	sess, err := h.store.ActiveSession(r.Context(), req.SessionID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if sess == nil {
		httpx.Error(w, http.StatusNotFound, "Edit session not found")
		return
	}
	if sess.UserID != user.ID {
		httpx.Error(w, http.StatusForbidden, "Not allowed to access this session")
		return
	}
	if sess.EditorType != editorMonaco {
		httpx.Error(w, http.StatusBadRequest, "Invalid session type")
		return
	}
	if sess.Mode == modeEdit {
		_, _ = h.releaseOwned(r, sess.Bucket, sess.Key, user.ID, nil)
	}
	if err := h.store.CloseSession(r.Context(), sess.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"closed": true})
}

func (h *Handler) loadTextSession(w http.ResponseWriter, r *http.Request, user *auth.User) *Session {
	sess, err := h.store.ActiveSession(r.Context(), r.PathValue("session_id"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil
	}
	if sess == nil {
		httpx.Error(w, http.StatusNotFound, "Edit session not found")
		return nil
	}
	if sess.UserID != user.ID {
		httpx.Error(w, http.StatusForbidden, "Not allowed to access this session")
		return nil
	}
	if sess.EditorType != editorMonaco {
		httpx.Error(w, http.StatusBadRequest, "Invalid session type")
		return nil
	}
	return sess
}

func (h *Handler) requireSessionLock(w http.ResponseWriter, r *http.Request, sess *Session) bool {
	lock, err := h.store.ActiveLock(r.Context(), sess.Bucket, sess.Key)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return false
	}
	if lock == nil {
		httpx.Error(w, http.StatusConflict, "Edit lock is no longer active")
		return false
	}
	if lock.LockedBy != sess.UserID {
		httpx.Error(w, http.StatusConflict, "File is locked by another user")
		return false
	}
	if !tokenOK(lock.Token, sess.CallbackToken) {
		httpx.Error(w, http.StatusForbidden, "Lock token mismatch")
		return false
	}
	return true
}

func decodeText(raw []byte) string {
	if utf8.Valid(raw) {
		return string(raw)
	}
	return strings.ToValidUTF8(string(raw), "\uFFFD")
}
