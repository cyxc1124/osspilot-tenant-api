package rbac

import (
	"net/http"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type groupCreateReq struct {
	TenantID    *int64  `json:"tenant_id"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
}

type groupUpdateReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type membersReq struct {
	UserIDs []int64 `json:"user_ids"`
}

type memberJSON struct {
	UserID      int64   `json:"user_id"`
	Username    string  `json:"username"`
	DisplayName *string `json:"display_name"`
}

type groupJSON struct {
	ID          int64        `json:"id"`
	TenantID    int64        `json:"tenant_id"`
	Name        string       `json:"name"`
	Description *string      `json:"description"`
	MemberCount int          `json:"member_count"`
	Members     []memberJSON `json:"members"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
}

func (h *Handler) listGroups(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	items, err := h.store.ListGroups(r.Context(), accountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]groupJSON, 0, len(items))
	for _, g := range items {
		out = append(out, toGroupJSON(g))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createGroup(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) || !h.requireAdmin(w, r, user) {
		return
	}
	var req groupCreateReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	accountID := auth.AccountID(user)
	if req.TenantID != nil && *req.TenantID != accountID {
		httpx.Error(w, http.StatusForbidden, "Tenant access denied")
		return
	}
	if req.Name == "" || len(req.Name) > 64 {
		httpx.Error(w, http.StatusBadRequest, "name is required")
		return
	}
	g, err := h.store.InsertGroup(r.Context(), accountID, req.Name, emptyToNil(req.Description))
	if err != nil {
		if isUnique(err) {
			httpx.Error(w, http.StatusConflict, "Group name already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusCreated, toGroupJSON(*g))
}

func (h *Handler) getGroup(w http.ResponseWriter, r *http.Request, user *auth.User) {
	_, g, ok := h.loadGroup(w, r, user)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, toGroupJSON(*g))
}

func (h *Handler) updateGroup(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, g, ok := h.loadGroup(w, r, user)
	if !ok {
		return
	}
	var req groupUpdateReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.store.UpdateGroup(r.Context(), accountID, g.ID, req.Name, req.Description); err != nil {
		if isUnique(err) {
			httpx.Error(w, http.StatusConflict, "Group name already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	updated, err := h.store.GetGroup(r.Context(), accountID, g.ID)
	if err != nil || updated == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, toGroupJSON(*updated))
}

func (h *Handler) deleteGroup(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, g, ok := h.loadGroup(w, r, user)
	if !ok {
		return
	}
	item := toGroupJSON(*g)
	if err := h.store.DeleteGroup(r.Context(), accountID, g.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handler) addMembers(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, g, ok := h.loadGroup(w, r, user)
	if !ok {
		return
	}
	var req membersReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if len(req.UserIDs) == 0 {
		httpx.Error(w, http.StatusBadRequest, "user_ids must not be empty")
		return
	}
	for _, uid := range req.UserIDs {
		u, err := h.users.GetInAccount(r.Context(), accountID, uid)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if u == nil {
			httpx.Error(w, http.StatusBadRequest, "User not found")
			return
		}
	}
	if err := h.store.AddMembers(r.Context(), g.ID, req.UserIDs); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	updated, err := h.store.GetGroup(r.Context(), accountID, g.ID)
	if err != nil || updated == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, toGroupJSON(*updated))
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, g, ok := h.loadGroup(w, r, user)
	if !ok {
		return
	}
	uid, ok := pathID(r, "user_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid user_id")
		return
	}
	if err := h.store.RemoveMember(r.Context(), g.ID, uid); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	updated, err := h.store.GetGroup(r.Context(), accountID, g.ID)
	if err != nil || updated == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, toGroupJSON(*updated))
}

func (h *Handler) loadGroup(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, *Group, bool) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return 0, nil, false
	}
	id, ok := pathID(r, "group_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid group_id")
		return 0, nil, false
	}
	g, err := h.store.GetGroup(r.Context(), accountID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return 0, nil, false
	}
	if g == nil {
		httpx.Error(w, http.StatusNotFound, "User group not found")
		return 0, nil, false
	}
	return accountID, g, true
}

func toGroupJSON(g Group) groupJSON {
	members := make([]memberJSON, 0, len(g.Members))
	for _, m := range g.Members {
		members = append(members, memberJSON{UserID: m.UserID, Username: m.Username, DisplayName: m.DisplayName})
	}
	return groupJSON{
		ID: g.ID, TenantID: g.AccountID, Name: g.Name, Description: g.Description,
		MemberCount: len(members), Members: members,
		CreatedAt: rfc3339(g.CreatedAt), UpdatedAt: rfc3339(g.UpdatedAt),
	}
}
