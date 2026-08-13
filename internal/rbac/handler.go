package rbac

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type bucketIDs interface {
	VisibleID(ctx context.Context, accountID int64, name string) (*int64, error)
}

type Handler struct {
	users   *auth.Store
	store   *Store
	buckets bucketIDs
	protect func(auth.UserHandler) http.HandlerFunc
	ac      *Checker
}

func NewHandler(users *auth.Store, store *Store, buckets bucketIDs, protect func(auth.UserHandler) http.HandlerFunc) *Handler {
	return &Handler{users: users, store: store, buckets: buckets, protect: protect, ac: NewChecker(store)}
}

func (h *Handler) Checker() *Checker { return h.ac }

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/roles", h.protect(h.listRoles))
	mux.HandleFunc("GET /api/users", h.protect(h.listUsers))
	mux.HandleFunc("POST /api/users", h.protect(h.createUser))
	mux.HandleFunc("GET /api/users/{user_id}", h.protect(h.getUser))
	mux.HandleFunc("PUT /api/users/{user_id}", h.protect(h.updateUser))
	mux.HandleFunc("DELETE /api/users/{user_id}", h.protect(h.deleteUser))
	mux.HandleFunc("GET /api/user-groups", h.protect(h.listGroups))
	mux.HandleFunc("POST /api/user-groups", h.protect(h.createGroup))
	mux.HandleFunc("GET /api/user-groups/{group_id}", h.protect(h.getGroup))
	mux.HandleFunc("PUT /api/user-groups/{group_id}", h.protect(h.updateGroup))
	mux.HandleFunc("DELETE /api/user-groups/{group_id}", h.protect(h.deleteGroup))
	mux.HandleFunc("POST /api/user-groups/{group_id}/members", h.protect(h.addMembers))
	mux.HandleFunc("DELETE /api/user-groups/{group_id}/members/{user_id}", h.protect(h.removeMember))
	mux.HandleFunc("GET /api/permissions", h.protect(h.listPermissions))
	mux.HandleFunc("POST /api/permissions", h.protect(h.createPermission))
	mux.HandleFunc("PUT /api/permissions/{permission_id}", h.protect(h.updatePermission))
	mux.HandleFunc("DELETE /api/permissions/{permission_id}", h.protect(h.deletePermission))
	mux.HandleFunc("GET /api/permission-templates", h.protect(h.listTemplates))
	mux.HandleFunc("POST /api/permission-templates", h.protect(h.createTemplate))
	mux.HandleFunc("GET /api/permission-templates/{template_id}", h.protect(h.getTemplate))
	mux.HandleFunc("PUT /api/permission-templates/{template_id}", h.protect(h.updateTemplate))
	mux.HandleFunc("DELETE /api/permission-templates/{template_id}", h.protect(h.deleteTemplate))
	mux.HandleFunc("POST /api/permission-templates/{template_id}/rules", h.protect(h.createTemplateRule))
	mux.HandleFunc("PUT /api/permission-templates/{template_id}/rules/{rule_id}", h.protect(h.updateTemplateRule))
	mux.HandleFunc("DELETE /api/permission-templates/{template_id}/rules/{rule_id}", h.protect(h.deleteTemplateRule))
	mux.HandleFunc("POST /api/permission-templates/{template_id}/assignments", h.protect(h.createAssignment))
	mux.HandleFunc("DELETE /api/permission-templates/{template_id}/assignments/{assignment_id}", h.protect(h.deleteAssignment))
}

func (h *Handler) ready(w http.ResponseWriter) bool {
	if h.users == nil || h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return false
	}
	return true
}

func (h *Handler) account(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, bool) {
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

func (h *Handler) requireAdmin(w http.ResponseWriter, r *http.Request, user *auth.User) bool {
	return !h.ac.Forbidden(w, r, user, "", "", ActionAdmin)
}

func (h *Handler) listRoles(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) || !h.requireAdmin(w, r, user) {
		return
	}
	items, err := h.store.ListRoles(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{"id": item.ID, "name": item.Name, "description": item.Description})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

type userCreateReq struct {
	Username    string  `json:"username"`
	Password    string  `json:"password"`
	DisplayName *string `json:"display_name"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	TenantID    *int64  `json:"tenant_id"`
	Role        string  `json:"role"`
}

type userUpdateReq struct {
	DisplayName *string `json:"display_name"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	Status      *string `json:"status"`
	Role        *string `json:"role"`
	Password    *string `json:"password"`
}

type roleInfo struct {
	Role     string `json:"role"`
	TenantID int64  `json:"tenant_id"`
}

type userJSON struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	DisplayName *string    `json:"display_name"`
	Email       *string    `json:"email"`
	Phone       *string    `json:"phone"`
	Status      string     `json:"status"`
	Roles       []roleInfo `json:"roles"`
	CreatedAt   string     `json:"created_at"`
	LastLoginAt *string    `json:"last_login_at"`
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	items, err := h.users.ListByAccount(r.Context(), accountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]userJSON, 0, len(items))
	for _, u := range items {
		out = append(out, toUserJSON(u, accountID))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createUser(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) || !h.requireAdmin(w, r, user) {
		return
	}
	var req userCreateReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	accountID := auth.AccountID(user)
	if req.TenantID != nil && *req.TenantID != accountID {
		httpx.Error(w, http.StatusForbidden, "Tenant access denied")
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" || len(req.Username) > 64 {
		httpx.Error(w, http.StatusBadRequest, "username is required")
		return
	}
	if len(req.Password) < 8 || len(req.Password) > 128 {
		httpx.Error(w, http.StatusBadRequest, "password must be 8-128 characters")
		return
	}
	role := req.Role
	if role == "" {
		role = auth.RoleNormalUser
	}
	if !auth.ValidRole(role) {
		httpx.Error(w, http.StatusBadRequest, "Invalid tenant role: '"+role+"'")
		return
	}
	if existing, err := h.users.GetByUsername(r.Context(), req.Username); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	} else if existing != nil {
		httpx.Error(w, http.StatusConflict, "Username already exists")
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "hash error")
		return
	}
	created, err := h.users.InsertMember(r.Context(), accountID, req.Username, hash, emptyToNil(req.DisplayName), emptyToNil(req.Email), emptyToNil(req.Phone), role)
	if err != nil {
		if isUnique(err) {
			httpx.Error(w, http.StatusConflict, "Username already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusCreated, toUserJSON(*created, accountID))
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, target, ok := h.loadUser(w, r, user)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, toUserJSON(*target, accountID))
}

func (h *Handler) updateUser(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, target, ok := h.loadUser(w, r, user)
	if !ok {
		return
	}
	var req userUpdateReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "disabled" {
		httpx.Error(w, http.StatusBadRequest, "status must be active or disabled")
		return
	}
	if req.Role != nil && !auth.ValidRole(*req.Role) {
		httpx.Error(w, http.StatusBadRequest, "Invalid tenant role: '"+*req.Role+"'")
		return
	}
	var hash *string
	if req.Password != nil {
		if len(*req.Password) < 8 || len(*req.Password) > 128 {
			httpx.Error(w, http.StatusBadRequest, "password must be 8-128 characters")
			return
		}
		hsh, err := auth.HashPassword(*req.Password)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "hash error")
			return
		}
		hash = &hsh
	}
	if err := h.users.UpdateMember(r.Context(), target.ID, req.DisplayName, req.Email, req.Phone, req.Status, req.Role, hash); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	updated, err := h.users.GetInAccount(r.Context(), accountID, target.ID)
	if err != nil || updated == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, toUserJSON(*updated, accountID))
}

func (h *Handler) deleteUser(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, target, ok := h.loadUser(w, r, user)
	if !ok {
		return
	}
	if target.ID == user.ID {
		httpx.Error(w, http.StatusBadRequest, "Cannot remove yourself from the tenant")
		return
	}
	if target.ID == accountID {
		httpx.Error(w, http.StatusBadRequest, "Cannot remove the account owner")
		return
	}
	item := toUserJSON(*target, accountID)
	if err := h.users.DeleteInAccount(r.Context(), accountID, target.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handler) gate(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, bool) {
	if !h.ready(w) || !h.requireAdmin(w, r, user) {
		return 0, false
	}
	return h.account(w, r, user)
}

func (h *Handler) loadUser(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, *auth.User, bool) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return 0, nil, false
	}
	id, err := strconv.ParseInt(r.PathValue("user_id"), 10, 64)
	if err != nil || id < 1 {
		httpx.Error(w, http.StatusBadRequest, "invalid user_id")
		return 0, nil, false
	}
	target, err := h.users.GetInAccount(r.Context(), accountID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return 0, nil, false
	}
	if target == nil {
		httpx.Error(w, http.StatusNotFound, "User not found")
		return 0, nil, false
	}
	return accountID, target, true
}

func toUserJSON(u auth.User, accountID int64) userJSON {
	return userJSON{
		ID: u.ID, Username: u.Username, DisplayName: u.DisplayName, Email: u.Email, Phone: u.Phone,
		Status: u.Status, Roles: []roleInfo{{Role: u.Role, TenantID: accountID}},
		CreatedAt: rfc3339(u.CreatedAt), LastLoginAt: rfc3339p(u.LastLoginAt),
	}
}

func rfc3339(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func rfc3339p(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

func emptyToNil(s *string) *string {
	if s == nil || strings.TrimSpace(*s) == "" {
		return nil
	}
	return s
}

func pathID(r *http.Request, name string) (int64, bool) {
	n, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return n, err == nil && n > 0
}
