package rbac

import (
	"net/http"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type permCreateReq struct {
	TenantID   *int64   `json:"tenant_id"`
	UserID     *int64   `json:"user_id"`
	RoleID     *int64   `json:"role_id"`
	GroupID    *int64   `json:"group_id"`
	BucketName *string  `json:"bucket_name"`
	Prefix     *string  `json:"prefix"`
	Actions    []string `json:"actions"`
}

type permUpdateReq struct {
	BucketName *string  `json:"bucket_name"`
	Prefix     *string  `json:"prefix"`
	Actions    []string `json:"actions"`
}

type permJSON struct {
	ID         int64    `json:"id"`
	TenantID   int64    `json:"tenant_id"`
	UserID     *int64   `json:"user_id"`
	RoleID     *int64   `json:"role_id"`
	RoleName   *string  `json:"role_name"`
	GroupID    *int64   `json:"group_id"`
	GroupName  *string  `json:"group_name"`
	BucketID   *int64   `json:"bucket_id"`
	BucketName *string  `json:"bucket_name"`
	Prefix     *string  `json:"prefix"`
	Actions    []string `json:"actions"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

func (h *Handler) listPermissions(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	items, err := h.store.ListPermissions(r.Context(), accountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]permJSON, 0, len(items))
	for _, p := range items {
		out = append(out, toPermJSON(p))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createPermission(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) || !h.requireAdmin(w, r, user) {
		return
	}
	var req permCreateReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	accountID := auth.AccountID(user)
	if req.TenantID != nil && *req.TenantID != accountID {
		httpx.Error(w, http.StatusForbidden, "Tenant access denied")
		return
	}
	n := 0
	if req.UserID != nil {
		n++
	}
	if req.RoleID != nil {
		n++
	}
	if req.GroupID != nil {
		n++
	}
	if n != 1 {
		httpx.Error(w, http.StatusBadRequest, "Specify exactly one of user_id, role_id, or group_id")
		return
	}
	if msg := ValidateActions(req.Actions); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return
	}
	if msg := scopeOK(req.Actions, req.BucketName); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return
	}
	p := Permission{AccountID: accountID, UserID: req.UserID, RoleID: req.RoleID, GroupID: req.GroupID, Prefix: emptyToNil(req.Prefix), Actions: req.Actions}
	if req.UserID != nil {
		u, err := h.users.GetInAccount(r.Context(), accountID, *req.UserID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if u == nil {
			httpx.Error(w, http.StatusBadRequest, "user_id does not exist for tenant")
			return
		}
	}
	if req.RoleID != nil {
		role, err := h.store.RoleByID(r.Context(), *req.RoleID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if role == nil {
			httpx.Error(w, http.StatusBadRequest, "role_id does not exist")
			return
		}
	}
	if req.GroupID != nil {
		g, err := h.store.GetGroup(r.Context(), accountID, *req.GroupID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if g == nil {
			httpx.Error(w, http.StatusBadRequest, "group_id does not exist for tenant")
			return
		}
	}
	bid, ok := h.resolveBucket(w, r, user, req.BucketName)
	if !ok {
		return
	}
	p.BucketID = bid
	created, err := h.store.InsertPermission(r.Context(), p)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusCreated, toPermJSON(*created))
}

func (h *Handler) updatePermission(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	id, ok := pathID(r, "permission_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid permission_id")
		return
	}
	cur, err := h.store.GetPermission(r.Context(), accountID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if cur == nil {
		httpx.Error(w, http.StatusNotFound, "TenantPermission rule not found")
		return
	}
	var req permUpdateReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	bucketID := cur.BucketID
	prefix := cur.Prefix
	actions := cur.Actions
	if req.BucketName != nil {
		bid, ok := h.resolveBucket(w, r, user, req.BucketName)
		if !ok {
			return
		}
		bucketID = bid
	}
	if req.Prefix != nil {
		prefix = emptyToNil(req.Prefix)
	}
	if req.Actions != nil {
		if msg := ValidateActions(req.Actions); msg != "" {
			httpx.Error(w, http.StatusBadRequest, msg)
			return
		}
		actions = req.Actions
	}
	var bname *string
	if req.BucketName != nil {
		bname = req.BucketName
	} else {
		bname = cur.BucketName
	}
	if msg := scopeOK(actions, bname); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return
	}
	if err := h.store.UpdatePermission(r.Context(), accountID, id, bucketID, prefix, actions); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	updated, err := h.store.GetPermission(r.Context(), accountID, id)
	if err != nil || updated == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, toPermJSON(*updated))
}

func (h *Handler) deletePermission(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	id, ok := pathID(r, "permission_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid permission_id")
		return
	}
	cur, err := h.store.GetPermission(r.Context(), accountID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if cur == nil {
		httpx.Error(w, http.StatusNotFound, "TenantPermission rule not found")
		return
	}
	item := toPermJSON(*cur)
	if err := h.store.DeletePermission(r.Context(), accountID, id); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handler) resolveBucket(w http.ResponseWriter, r *http.Request, user *auth.User, name *string) (*int64, bool) {
	if name == nil || *name == "" {
		return nil, true
	}
	if h.buckets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
		return nil, false
	}
	id, err := h.buckets.VisibleID(r.Context(), auth.AccountID(user), *name)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	if id == nil {
		httpx.Error(w, http.StatusBadRequest, "Bucket not found")
		return nil, false
	}
	return id, true
}

func scopeOK(actions []string, bucketName *string) string {
	if bucketName == nil || *bucketName == "" {
		return ""
	}
	for _, a := range actions {
		if a == ActionBucketCreate {
			return "bucket_create must use account-level scope (leave bucket empty)"
		}
	}
	return ""
}

func toPermJSON(p Permission) permJSON {
	return permJSON{
		ID: p.ID, TenantID: p.AccountID, UserID: p.UserID, RoleID: p.RoleID, RoleName: p.RoleName,
		GroupID: p.GroupID, GroupName: p.GroupName, BucketID: p.BucketID, BucketName: p.BucketName,
		Prefix: p.Prefix, Actions: p.Actions, CreatedAt: rfc3339(p.CreatedAt), UpdatedAt: rfc3339(p.UpdatedAt),
	}
}
