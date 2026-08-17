package edit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

const (
	officeStatusEditing        = 1
	officeStatusReadyToSave    = 2
	officeStatusSaveError      = 3
	officeStatusClosedNoChange = 4
	officeStatusForceSave      = 6
	officeStatusForceSaveError = 7
)

type officeOpenReq struct {
	BucketName string `json:"bucket_name"`
	ObjectKey  string `json:"object_key"`
	TenantID   *int64 `json:"tenant_id"`
	Mode       string `json:"mode"`
}

type officeSaveReq struct {
	SessionID string `json:"session_id"`
}

func (h *Handler) officeOpen(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireStore(w) || !h.requireS3(w) {
		return
	}
	officeURL := h.officeURL(r)
	if officeURL == "" {
		httpx.Error(w, http.StatusServiceUnavailable, "OFFICE_URL is not configured")
		return
	}
	var req officeOpenReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	kind, okExt := officeExts[fileExt(req.ObjectKey)]
	if !okExt {
		httpx.Error(w, http.StatusBadRequest, "Only docx, xlsx, and pptx files are supported for Office editing")
		return
	}
	b, ok := h.visible(w, r, user, req.BucketName, req.ObjectKey)
	if !ok {
		return
	}
	if _, err := h.client(r).HeadObject(r.Context(), b.BucketName, req.ObjectKey); err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	existing, err := h.store.ActiveLock(r.Context(), b.BucketName, req.ObjectKey)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	readonly := existing != nil && existing.LockedBy != user.ID
	mode := req.Mode
	if mode != modeEdit && mode != modeView {
		mode = modeEdit
	}
	if readonly {
		mode = modeView
	}
	sid, err := newSessionID()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	docKey, err := newDocumentKey(b.BucketName)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	cbToken, err := newToken()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	expires := time.Now().UTC().Add(sessionTTL)
	if !readonly {
		if _, _, err := h.acquire(r, b.BucketName, req.ObjectKey, user.ID, cbToken); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
	}
	sess := &Session{
		ID:            sid,
		Bucket:        b.BucketName,
		Key:           req.ObjectKey,
		UserID:        user.ID,
		EditorType:    editorOffice,
		Mode:          mode,
		DocumentKey:   &docKey,
		CallbackToken: cbToken,
		ExpiresAt:     expires,
	}
	if err := h.store.InsertSession(r.Context(), sess); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	cfg, err := h.officeConfig(sid, cbToken, docKey, req.ObjectKey, kind, user, mode)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"session_id": sid,
		"office_url": officeURL,
		"config":     cfg,
		"locked":     !readonly,
		"readonly":   readonly,
		"expires_at": formatTime(expires),
	})
}

func (h *Handler) officeDownload(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) || !h.requireS3(w) {
		return
	}
	sess, err := h.store.ActiveSession(r.Context(), r.PathValue("session_id"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if sess == nil {
		httpx.Error(w, http.StatusNotFound, "Edit session not found")
		return
	}
	if !tokenOK(sess.CallbackToken, r.URL.Query().Get("token")) {
		httpx.Error(w, http.StatusForbidden, "Invalid download token")
		return
	}
	kind, ok := officeExts[fileExt(sess.Key)]
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "Unsupported file type")
		return
	}
	body, _, err := h.client(r).GetObject(r.Context(), sess.Bucket, sess.Key, 0)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			httpx.Error(w, http.StatusNotFound, "Object not found")
			return
		}
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	name := path.Base(sess.Key)
	w.Header().Set("Content-Type", kind.ContentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *Handler) officeCallback(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) || !h.requireS3(w) {
		return
	}
	sess, err := h.store.ActiveSession(r.Context(), r.PathValue("session_id"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if sess == nil {
		httpx.Error(w, http.StatusNotFound, "Edit session not found")
		return
	}
	if !tokenOK(sess.CallbackToken, r.URL.Query().Get("token")) {
		httpx.Error(w, http.StatusForbidden, "Invalid callback token")
		return
	}
	var body map[string]any
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	payload, err := h.parseOfficeCallback(body)
	if err != nil {
		var se statusErr
		if errors.As(err, &se) {
			httpx.Error(w, se.status, se.msg)
			return
		}
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	status, _ := intFrom(payload["status"])
	if status == 0 && payload["status"] == nil {
		httpx.Error(w, http.StatusBadRequest, "Missing callback status")
		return
	}
	if key, _ := payload["key"].(string); key != "" && sess.DocumentKey != nil && key != *sess.DocumentKey {
		matched, err := h.store.SessionByDocumentKey(r.Context(), key)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if matched == nil {
			httpx.JSON(w, http.StatusOK, map[string]int{"error": 1})
			return
		}
		sess = matched
	}
	switch status {
	case officeStatusEditing:
		httpx.JSON(w, http.StatusOK, map[string]int{"error": 0})
		return
	case officeStatusReadyToSave, officeStatusForceSave:
		url, _ := payload["url"].(string)
		if url == "" {
			httpx.Error(w, http.StatusBadRequest, "Callback missing document url")
			return
		}
		if err := h.saveFromURL(r, sess, url); err != nil {
			httpx.Error(w, http.StatusBadGateway, "Failed to download edited document")
			return
		}
		httpx.JSON(w, http.StatusOK, map[string]int{"error": 0})
		return
	case officeStatusSaveError, officeStatusForceSaveError, 0:
		httpx.JSON(w, http.StatusOK, map[string]int{"error": 1})
		return
	case officeStatusClosedNoChange:
		_, _ = h.releaseOwned(r, sess.Bucket, sess.Key, sess.UserID, &sess.CallbackToken)
		_ = h.store.CloseSession(r.Context(), sess.ID)
		httpx.JSON(w, http.StatusOK, map[string]int{"error": 0})
		return
	default:
		httpx.JSON(w, http.StatusOK, map[string]int{"error": 0})
	}
}

func (h *Handler) officeSave(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.requireStore(w) {
		return
	}
	var req officeSaveReq
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
		httpx.Error(w, http.StatusForbidden, "Not allowed to save this session")
		return
	}
	if sess.LastSavedAt == nil {
		httpx.Error(w, http.StatusConflict, "No saved version available yet")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"saved":    true,
		"etag":     sess.LastETag,
		"saved_at": formatTimePtr(sess.LastSavedAt),
	})
}

func (h *Handler) officeUnlock(w http.ResponseWriter, r *http.Request, user *auth.User) {
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
		lock, _ := h.store.ActiveLock(r.Context(), b.BucketName, req.ObjectKey)
		if lock == nil {
			httpx.JSON(w, http.StatusOK, map[string]any{"unlocked": true})
			return
		}
		if lock.LockedBy != user.ID {
			httpx.Error(w, http.StatusForbidden, "Not allowed to unlock this file")
			return
		}
		httpx.Error(w, http.StatusForbidden, "Invalid lock token")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"unlocked": true})
}

func (h *Handler) officeConfig(sessionID, cbToken, docKey, objectKey string, kind officeKind, user *auth.User, mode string) (map[string]any, error) {
	base := h.office.PublicURL
	download := fmt.Sprintf("%s/api/editor/files/%s/download?token=%s", base, sessionID, cbToken)
	callback := fmt.Sprintf("%s/api/editor/callback/%s?token=%s", base, sessionID, cbToken)
	name := user.Username
	if user.DisplayName != nil && *user.DisplayName != "" {
		name = *user.DisplayName
	}
	cfg := map[string]any{
		"document": map[string]any{
			"fileType": kind.FileType,
			"key":      docKey,
			"title":    path.Base(objectKey),
			"url":      download,
		},
		"documentType": kind.DocType,
		"editorConfig": map[string]any{
			"callbackUrl": callback,
			"mode":        mode,
			"user": map[string]any{
				"id":   strconv.FormatInt(user.ID, 10),
				"name": name,
			},
		},
	}
	if h.office.JWTSecret == "" {
		return cfg, nil
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"document":     cfg["document"],
		"documentType": cfg["documentType"],
		"editorConfig": cfg["editorConfig"],
	})
	signed, err := tok.SignedString([]byte(h.office.JWTSecret))
	if err != nil {
		return nil, err
	}
	cfg["token"] = signed
	return cfg, nil
}

func (h *Handler) parseOfficeCallback(body map[string]any) (map[string]any, error) {
	if h.office.JWTSecret == "" {
		return body, nil
	}
	raw, _ := body["token"].(string)
	if raw == "" {
		return nil, statusErr{http.StatusUnauthorized, "Missing ONLYOFFICE callback token"}
	}
	tok, err := jwt.Parse(raw, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("unexpected alg")
		}
		return []byte(h.office.JWTSecret), nil
	})
	if err != nil || !tok.Valid {
		return nil, statusErr{http.StatusUnauthorized, "Invalid ONLYOFFICE callback token"}
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, statusErr{http.StatusBadRequest, "Invalid ONLYOFFICE callback payload"}
	}
	return map[string]any(claims), nil
}

func (h *Handler) saveFromURL(r *http.Request, sess *Session, url string) error {
	kind, ok := officeExts[fileExt(sess.Key)]
	if !ok {
		return errors.New("unsupported")
	}
	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("office download %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	etag, err := h.client(r).PutObject(r.Context(), sess.Bucket, sess.Key, bytes.NewReader(body), kind.ContentType)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	return h.store.MarkSaved(r.Context(), sess.ID, etag, now)
}

func (h *Handler) officeURL(r *http.Request) string {
	if h.settings != nil {
		rows, err := h.settings.Map(r.Context())
		if err == nil {
			if v := strings.TrimSpace(rows["office_url"]); v != "" {
				return strings.TrimRight(v, "/")
			}
		}
	}
	return h.office.URL
}

type statusErr struct {
	status int
	msg    string
}

func (e statusErr) Error() string { return e.msg }

func intFrom(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	case string:
		i, err := strconv.Atoi(n)
		return i, err == nil
	}
	return 0, false
}
