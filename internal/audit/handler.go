package audit

import (
	"net/http"
	"strconv"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
)

type Handler struct {
	store   *Store
	protect func(auth.UserHandler) http.HandlerFunc
	ac      *rbac.Checker
}

func NewHandler(store *Store, protect func(auth.UserHandler) http.HandlerFunc, ac *rbac.Checker) *Handler {
	return &Handler{store: store, protect: protect, ac: ac}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/audit-logs", h.protect(h.list))
	mux.HandleFunc("GET /api/audit-logs/export", h.protect(h.export))
}

func (h *Handler) ready(w http.ResponseWriter) bool {
	if h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return false
	}
	return true
}

func (h *Handler) allow(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, bool) {
	if !h.ready(w) {
		return 0, false
	}
	accountID := auth.AccountID(user)
	if raw := r.URL.Query().Get("tenant_id"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n != accountID {
			httpx.Error(w, http.StatusForbidden, "Tenant access denied")
			return 0, false
		}
	}
	if auth.IsAdmin(user) || user.Role == auth.RoleAuditUser {
		return accountID, true
	}
	if h.ac.Forbidden(w, r, user, "", "", rbac.ActionAudit) {
		return 0, false
	}
	return accountID, true
}

func (h *Handler) parseFilter(w http.ResponseWriter, r *http.Request, accountID int64) (Filter, bool) {
	q := r.URL.Query()
	f := Filter{AccountID: accountID, BucketName: q.Get("bucket_name"), ObjectKey: q.Get("object_key"), Action: q.Get("action"), Status: q.Get("status"), SourceIP: q.Get("source_ip"), Keyword: q.Get("keyword")}
	if raw := q.Get("user_id"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			httpx.Error(w, http.StatusBadRequest, "user_id must be an integer")
			return Filter{}, false
		}
		f.UserID = &n
	}
	switch q.Get("admin_only") {
	case "", "false", "0":
	case "true", "1":
		f.AdminOnly = true
	default:
		httpx.Error(w, http.StatusBadRequest, "admin_only must be a boolean")
		return Filter{}, false
	}
	from, ok := parseTime(w, q.Get("created_from"), "created_from")
	if !ok {
		return Filter{}, false
	}
	to, ok := parseTime(w, q.Get("created_to"), "created_to")
	if !ok {
		return Filter{}, false
	}
	f.CreatedFrom, f.CreatedTo = from, to
	return f, true
}

func parseTime(w http.ResponseWriter, raw, name string) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, name+" must be RFC3339")
		return nil, false
	}
	return &t, true
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.allow(w, r, user)
	if !ok {
		return
	}
	f, ok := h.parseFilter(w, r, accountID)
	if !ok {
		return
	}
	page, pageSize := 1, 20
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			httpx.Error(w, http.StatusBadRequest, "page must be >= 1")
			return
		}
		page = n
	}
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			httpx.Error(w, http.StatusBadRequest, "page_size must be 1-100")
			return
		}
		pageSize = n
	}
	items, total, err := h.store.List(r.Context(), f, page, pageSize)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, e := range items {
		out = append(out, entryJSON(e))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "page": page, "page_size": pageSize, "total": total})
}

func (h *Handler) export(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.allow(w, r, user)
	if !ok {
		return
	}
	f, ok := h.parseFilter(w, r, accountID)
	if !ok {
		return
	}
	items, err := h.store.Export(r.Context(), f)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-logs.csv"`)
	if err := writeCSV(w, items); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "export error")
	}
}

func entryJSON(e Entry) map[string]any {
	return map[string]any{
		"id": e.ID, "user_id": e.UserID, "username": e.Username, "tenant_id": e.TenantID, "tenant_name": e.TenantName,
		"bucket_name": e.BucketName, "object_key": e.ObjectKey, "action": e.Action, "source_ip": e.SourceIP,
		"user_agent": e.UserAgent, "status": e.Status, "error_message": e.ErrorMessage,
		"created_at": e.CreatedAt.UTC().Format(time.RFC3339),
	}
}
