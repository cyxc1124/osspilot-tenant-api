package rbac

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

func (h *Handler) registerInternal(mux *http.ServeMux) {
	wrap := func(fn auth.UserHandler) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			user, ok := h.gateInternal(w, r)
			if !ok {
				return
			}
			fn(w, r, user)
		}
	}
	const p = "/internal/accounts/{username}/rbac"
	mux.HandleFunc("GET "+p+"/user-groups", wrap(h.listGroups))
	mux.HandleFunc("POST "+p+"/user-groups", wrap(h.createGroup))
	mux.HandleFunc("PUT "+p+"/user-groups/{group_id}", wrap(h.updateGroup))
	mux.HandleFunc("DELETE "+p+"/user-groups/{group_id}", wrap(h.deleteGroup))
	mux.HandleFunc("POST "+p+"/user-groups/{group_id}/members", wrap(h.addMembers))
	mux.HandleFunc("DELETE "+p+"/user-groups/{group_id}/members/{user_id}", wrap(h.removeMember))
	mux.HandleFunc("GET "+p+"/permission-templates", wrap(h.listTemplates))
	mux.HandleFunc("POST "+p+"/permission-templates", wrap(h.createTemplate))
	mux.HandleFunc("PUT "+p+"/permission-templates/{template_id}", wrap(h.updateTemplate))
	mux.HandleFunc("DELETE "+p+"/permission-templates/{template_id}", wrap(h.deleteTemplate))
	mux.HandleFunc("POST "+p+"/permission-templates/{template_id}/rules", wrap(h.createTemplateRule))
	mux.HandleFunc("PUT "+p+"/permission-templates/{template_id}/rules/{rule_id}", wrap(h.updateTemplateRule))
	mux.HandleFunc("DELETE "+p+"/permission-templates/{template_id}/rules/{rule_id}", wrap(h.deleteTemplateRule))
	mux.HandleFunc("POST "+p+"/permission-templates/{template_id}/assignments", wrap(h.createAssignment))
	mux.HandleFunc("DELETE "+p+"/permission-templates/{template_id}/assignments/{assignment_id}", wrap(h.deleteAssignment))
	mux.HandleFunc("GET "+p+"/permissions", wrap(h.listPermissions))
	mux.HandleFunc("POST "+p+"/permissions", wrap(h.createPermission))
	mux.HandleFunc("PUT "+p+"/permissions/{permission_id}", wrap(h.updatePermission))
	mux.HandleFunc("DELETE "+p+"/permissions/{permission_id}", wrap(h.deletePermission))
}

func (h *Handler) gateInternal(w http.ResponseWriter, r *http.Request) (*auth.User, bool) {
	if h.secret == "" || h.users == nil || h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return nil, false
	}
	if !projectionOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return nil, false
	}
	username := strings.TrimSpace(r.PathValue("username"))
	if username == "" {
		httpx.Error(w, http.StatusBadRequest, "username is required")
		return nil, false
	}
	user, err := h.users.GetByUsername(r.Context(), username)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	if user == nil {
		httpx.Error(w, http.StatusNotFound, "Account not found")
		return nil, false
	}
	// ponytail: ops proxy acts as tenant_admin; reuse JWT handlers without a session.
	return &auth.User{
		ID: user.ID, Username: user.Username, AccountID: auth.AccountID(user),
		Role: auth.RoleTenantAdmin, Status: user.Status,
	}, true
}

func projectionOK(header, secret string) bool {
	const p = "Bearer "
	if secret == "" || !strings.HasPrefix(header, p) {
		return false
	}
	got := strings.TrimSpace(header[len(p):])
	if len(got) != len(secret) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}
