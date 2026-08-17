package creds

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/audit"
	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/platform"
	"github.com/cyxc1124/osspilot-tenant-api/internal/quota"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
	"github.com/cyxc1124/osspilot-tenant-api/internal/uploads"
)

type Handler struct {
	store    *Store
	users    *auth.Store
	protect  func(auth.UserHandler) http.HandlerFunc
	ac       *rbac.Checker
	s3End    string
	s3Reg    string
	secret   string
	buckets  *bucket.Store
	objects  *objects.Store
	uploads  *uploads.Store
	s3       *storage.Client
	s3fb     storage.Config
	settings *platform.Store
	log      *audit.Logger
	quota    *quota.Checker
}

func NewHandler(store *Store, users *auth.Store, protect func(auth.UserHandler) http.HandlerFunc, ac *rbac.Checker, s3Endpoint, s3Region, jwtSecret string, buckets *bucket.Store, objectStore *objects.Store, uploadStore *uploads.Store, s3 *storage.Client, log *audit.Logger, q *quota.Checker, settings *platform.Store, s3fb storage.Config) *Handler {
	return &Handler{store: store, users: users, protect: protect, ac: ac, s3End: s3Endpoint, s3Reg: s3Region, secret: jwtSecret, buckets: buckets, objects: objectStore, uploads: uploadStore, s3: s3, s3fb: s3fb, settings: settings, log: log, quota: q}
}

func (h *Handler) client(r *http.Request) *storage.Client {
	return storage.Live(r.Context(), h.s3fb, h.settings, h.s3)
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/api-access", h.protect(h.getAccess))
	mux.HandleFunc("POST /api/api-access/request", h.protect(h.requestAccess))
	mux.HandleFunc("GET /api/applications", h.protect(h.listApps))
	mux.HandleFunc("POST /api/applications", h.protect(h.createApp))
	mux.HandleFunc("PUT /api/applications/{application_id}", h.protect(h.updateApp))
	mux.HandleFunc("DELETE /api/applications/{application_id}", h.protect(h.deleteApp))
	mux.HandleFunc("GET /api/applications/{application_id}/access-keys", h.protect(h.listKeys))
	mux.HandleFunc("POST /api/applications/{application_id}/access-keys", h.protect(h.createKey))
	mux.HandleFunc("DELETE /api/applications/{application_id}/access-keys/{key_id}", h.protect(h.disableKey))
	mux.HandleFunc("POST /api/sts/credentials", h.protect(h.issueSTS))
	h.registerOpen(mux)
}

func (h *Handler) ready(w http.ResponseWriter) bool {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return false
	}
	return true
}

func (h *Handler) gate(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, bool) {
	if !h.ready(w) {
		return 0, false
	}
	if h.ac.Forbidden(w, r, user, "", "", rbac.ActionAdmin) {
		return 0, false
	}
	id := auth.AccountID(user)
	if raw := r.URL.Query().Get("tenant_id"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n != id {
			httpx.Error(w, http.StatusForbidden, "Tenant access denied")
			return 0, false
		}
	}
	return id, true
}

func (h *Handler) requireApproved(w http.ResponseWriter, r *http.Request, accountID int64) bool {
	row, err := h.store.GetAccess(r.Context(), accountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return false
	}
	if row == nil || row.Status != "approved" {
		httpx.Error(w, http.StatusForbidden, "Tenant API access is not approved")
		return false
	}
	return true
}

func accessJSON(accountID int64, row *AccessRow) map[string]any {
	status := "not_requested"
	out := map[string]any{
		"tenant_id": accountID, "status": status, "requested_at": nil, "reviewed_at": nil,
		"review_note": nil, "rgw_uid": nil,
	}
	if row != nil {
		out["status"] = row.Status
		out["requested_at"] = rfc3339p(row.RequestedAt)
		out["reviewed_at"] = rfc3339p(row.ReviewedAt)
		out["review_note"] = row.ReviewNote
		out["rgw_uid"] = row.RGWUID
	}
	return out
}

func (h *Handler) getAccess(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	row, err := h.store.GetAccess(r.Context(), accountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, accessJSON(accountID, row))
}

func (h *Handler) requestAccess(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	var req struct {
		TenantID *int64  `json:"tenant_id"`
		Note     *string `json:"note"`
	}
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	if req.TenantID != nil && *req.TenantID != accountID {
		httpx.Error(w, http.StatusForbidden, "Tenant access denied")
		return
	}
	cur, err := h.store.GetAccess(r.Context(), accountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if cur != nil {
		switch cur.Status {
		case "pending":
			httpx.Error(w, http.StatusConflict, "API access request already pending")
			return
		case "approved":
			httpx.Error(w, http.StatusConflict, "API access already approved")
			return
		}
	}
	row, err := h.store.RequestAccess(r.Context(), accountID, user.ID, req.Note)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	h.log.Record(r, user, "", "", "request_tenant_api_access", "success", "")
	httpx.JSON(w, http.StatusOK, accessJSON(accountID, row))
}

type appReq struct {
	TenantID    *int64  `json:"tenant_id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type appUpdateReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Status      *string `json:"status"`
}

func appJSON(a App) map[string]any {
	return map[string]any{
		"id": a.ID, "tenant_id": a.AccountID, "name": a.Name, "description": a.Description,
		"status": a.Status, "created_by": a.CreatedBy,
		"created_at": rfc3339(a.CreatedAt), "updated_at": rfc3339(a.UpdatedAt),
		"access_key_count": a.KeyCount,
	}
}

func (h *Handler) listApps(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok || !h.requireApproved(w, r, accountID) {
		return
	}
	items, err := h.store.ListApps(r.Context(), accountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, a := range items {
		out = append(out, appJSON(a))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (h *Handler) createApp(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok || !h.requireApproved(w, r, accountID) {
		return
	}
	var req appReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 128 {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	a, err := h.store.InsertApp(r.Context(), accountID, user.ID, req.Name, req.Description)
	if err != nil {
		if isUnique(err) {
			httpx.Error(w, http.StatusConflict, "Application name already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	h.log.Record(r, user, "", a.Name, "create_application", "success", "")
	httpx.JSON(w, http.StatusCreated, appJSON(*a))
}

func (h *Handler) updateApp(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, app, ok := h.loadApp(w, r, user)
	if !ok {
		return
	}
	var req appUpdateReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "disabled" {
		httpx.Error(w, http.StatusBadRequest, "status must be active or disabled")
		return
	}
	if err := h.store.UpdateApp(r.Context(), accountID, app.ID, req.Name, req.Description, req.Status); err != nil {
		if isUnique(err) {
			httpx.Error(w, http.StatusConflict, "Application name already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	updated, err := h.store.GetApp(r.Context(), accountID, app.ID)
	if err != nil || updated == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	h.log.Record(r, user, "", updated.Name, "update_application", "success", "")
	httpx.JSON(w, http.StatusOK, appJSON(*updated))
}

func (h *Handler) deleteApp(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, app, ok := h.loadApp(w, r, user)
	if !ok {
		return
	}
	if err := h.store.DeleteApp(r.Context(), accountID, app.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	h.log.Record(r, user, "", app.Name, "delete_application", "success", "")
	w.WriteHeader(http.StatusNoContent)
}

func keyJSON(k Key) map[string]any {
	return map[string]any{
		"id": k.ID, "application_id": k.ApplicationID, "access_key_id": k.AccessKeyID,
		"status": k.Status, "description": k.Description, "last_used_at": rfc3339p(k.LastUsedAt),
		"created_at": rfc3339(k.CreatedAt), "updated_at": rfc3339(k.UpdatedAt),
	}
}

func (h *Handler) listKeys(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, app, ok := h.loadApp(w, r, user)
	if !ok {
		return
	}
	items, err := h.store.ListKeys(r.Context(), accountID, app.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, k := range items {
		out = append(out, keyJSON(k))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (h *Handler) createKey(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, app, ok := h.loadApp(w, r, user)
	if !ok {
		return
	}
	if app.Status != "active" {
		httpx.Error(w, http.StatusConflict, "Application is disabled")
		return
	}
	var req struct {
		Description *string `json:"description"`
	}
	if r.ContentLength != 0 {
		if err := httpx.DecodeJSON(r, &req); err != nil {
			httpx.Error(w, http.StatusBadRequest, "invalid json")
			return
		}
	}
	kid, err := newAccessKeyID()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "key error")
		return
	}
	secret, err := newSecretKey()
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "key error")
		return
	}
	hash, err := auth.HashPassword(secret)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "hash error")
		return
	}
	k := &Key{ApplicationID: app.ID, AccountID: accountID, AccessKeyID: kid, SecretHash: hash, Description: req.Description, CreatedBy: user.ID}
	if err := h.store.InsertKey(r.Context(), k); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	body := keyJSON(*k)
	body["secret_access_key"] = secret
	h.log.Record(r, user, "", k.AccessKeyID, "create_access_key", "success", "")
	httpx.JSON(w, http.StatusCreated, body)
}

func (h *Handler) disableKey(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, app, ok := h.loadApp(w, r, user)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("key_id"), 10, 64)
	if err != nil || id < 1 {
		httpx.Error(w, http.StatusBadRequest, "invalid key_id")
		return
	}
	k, err := h.store.GetKey(r.Context(), accountID, app.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if k == nil {
		httpx.Error(w, http.StatusNotFound, "Access key not found")
		return
	}
	if err := h.store.DisableKey(r.Context(), accountID, app.ID, id); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	k.Status = "disabled"
	h.log.Record(r, user, "", k.AccessKeyID, "disable_access_key", "success", "")
	httpx.JSON(w, http.StatusOK, keyJSON(*k))
}

func (h *Handler) loadApp(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, *App, bool) {
	accountID, ok := h.gate(w, r, user)
	if !ok || !h.requireApproved(w, r, accountID) {
		return 0, nil, false
	}
	id, err := strconv.ParseInt(r.PathValue("application_id"), 10, 64)
	if err != nil || id < 1 {
		httpx.Error(w, http.StatusBadRequest, "invalid application_id")
		return 0, nil, false
	}
	app, err := h.store.GetApp(r.Context(), accountID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return 0, nil, false
	}
	if app == nil {
		httpx.Error(w, http.StatusNotFound, "Application not found")
		return 0, nil, false
	}
	return accountID, app, true
}

func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func rfc3339p(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}
