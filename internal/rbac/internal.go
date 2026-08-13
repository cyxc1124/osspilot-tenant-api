package rbac

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

// RegisterInternal exposes RBAC for ops via username + PROJECTION_SECRET.
// Handlers reuse the same admin paths as /api/*; account scope comes from the resolved user.
func (h *Handler) RegisterInternal(mux *http.ServeMux, secret string) {
	h.secret = secret
	wrap := h.asProjectedAdmin
	mux.HandleFunc("GET /internal/accounts/{username}/rbac/user-groups", wrap(h.listGroups))
	mux.HandleFunc("POST /internal/accounts/{username}/rbac/user-groups", wrap(h.createGroup))
	mux.HandleFunc("GET /internal/accounts/{username}/rbac/user-groups/{group_id}", wrap(h.getGroup))
	mux.HandleFunc("PUT /internal/accounts/{username}/rbac/user-groups/{group_id}", wrap(h.updateGroup))
	mux.HandleFunc("DELETE /internal/accounts/{username}/rbac/user-groups/{group_id}", wrap(h.deleteGroup))
	mux.HandleFunc("POST /internal/accounts/{username}/rbac/user-groups/{group_id}/members", wrap(h.addMembers))
	mux.HandleFunc("DELETE /internal/accounts/{username}/rbac/user-groups/{group_id}/members/{user_id}", wrap(h.removeMember))
	mux.HandleFunc("GET /internal/accounts/{username}/rbac/permission-templates", wrap(h.listTemplates))
	mux.HandleFunc("POST /internal/accounts/{username}/rbac/permission-templates", wrap(h.createTemplate))
	mux.HandleFunc("GET /internal/accounts/{username}/rbac/permission-templates/{template_id}", wrap(h.getTemplate))
	mux.HandleFunc("PUT /internal/accounts/{username}/rbac/permission-templates/{template_id}", wrap(h.updateTemplate))
	mux.HandleFunc("DELETE /internal/accounts/{username}/rbac/permission-templates/{template_id}", wrap(h.deleteTemplate))
	mux.HandleFunc("POST /internal/accounts/{username}/rbac/permission-templates/{template_id}/rules", wrap(h.createTemplateRule))
	mux.HandleFunc("PUT /internal/accounts/{username}/rbac/permission-templates/{template_id}/rules/{rule_id}", wrap(h.updateTemplateRule))
	mux.HandleFunc("DELETE /internal/accounts/{username}/rbac/permission-templates/{template_id}/rules/{rule_id}", wrap(h.deleteTemplateRule))
	mux.HandleFunc("POST /internal/accounts/{username}/rbac/permission-templates/{template_id}/assignments", wrap(h.createAssignment))
	mux.HandleFunc("DELETE /internal/accounts/{username}/rbac/permission-templates/{template_id}/assignments/{assignment_id}", wrap(h.deleteAssignment))
	mux.HandleFunc("GET /internal/accounts/{username}/rbac/permissions", wrap(h.listPermissions))
	mux.HandleFunc("POST /internal/accounts/{username}/rbac/permissions", wrap(h.createPermission))
	mux.HandleFunc("PUT /internal/accounts/{username}/rbac/permissions/{permission_id}", wrap(h.updatePermission))
	mux.HandleFunc("DELETE /internal/accounts/{username}/rbac/permissions/{permission_id}", wrap(h.deletePermission))
}

func (h *Handler) asProjectedAdmin(fn auth.UserHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.secret == "" || h.users == nil || h.store == nil {
			httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
			return
		}
		if !projectionBearerOK(r.Header.Get("Authorization"), h.secret) {
			httpx.Error(w, http.StatusUnauthorized, "invalid token")
			return
		}
		username := strings.TrimSpace(r.PathValue("username"))
		if username == "" {
			httpx.Error(w, http.StatusBadRequest, "username is required")
			return
		}
		user, err := h.users.GetByUsername(r.Context(), username)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if user == nil {
			httpx.Error(w, http.StatusNotFound, "Account not found")
			return
		}
		fn(w, r, user)
	}
}

func projectionBearerOK(header, secret string) bool {
	const p = "Bearer "
	if secret == "" || !strings.HasPrefix(header, p) {
		return false
	}
	got := strings.TrimSpace(strings.TrimPrefix(header, p))
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}
