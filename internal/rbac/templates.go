package rbac

import (
	"net/http"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type ruleInput struct {
	BucketName *string  `json:"bucket_name"`
	Prefix     *string  `json:"prefix"`
	Actions    []string `json:"actions"`
}

type templateCreateReq struct {
	TenantID    *int64      `json:"tenant_id"`
	Name        string      `json:"name"`
	Description *string     `json:"description"`
	Rules       []ruleInput `json:"rules"`
}

type templateUpdateReq struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

type assignmentReq struct {
	UserID  *int64 `json:"user_id"`
	GroupID *int64 `json:"group_id"`
}

type ruleJSON struct {
	ID         int64    `json:"id"`
	TemplateID int64    `json:"template_id"`
	BucketID   *int64   `json:"bucket_id"`
	BucketName *string  `json:"bucket_name"`
	Prefix     *string  `json:"prefix"`
	Actions    []string `json:"actions"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

type assignmentJSON struct {
	ID         int64  `json:"id"`
	TenantID   int64  `json:"tenant_id"`
	TemplateID int64  `json:"template_id"`
	UserID     *int64 `json:"user_id"`
	GroupID    *int64 `json:"group_id"`
}

type templateJSON struct {
	ID          int64            `json:"id"`
	TenantID    int64            `json:"tenant_id"`
	Name        string           `json:"name"`
	Description *string          `json:"description"`
	Rules       []ruleJSON       `json:"rules"`
	Assignments []assignmentJSON `json:"assignments"`
	CreatedAt   string           `json:"created_at"`
	UpdatedAt   string           `json:"updated_at"`
}

func (h *Handler) listTemplates(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	items, err := h.store.ListTemplates(r.Context(), accountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]templateJSON, 0, len(items))
	for _, t := range items {
		out = append(out, toTemplateJSON(t))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) createTemplate(w http.ResponseWriter, r *http.Request, user *auth.User) {
	if !h.ready(w) || !h.requireAdmin(w, r, user) {
		return
	}
	var req templateCreateReq
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
	t, err := h.store.InsertTemplate(r.Context(), accountID, req.Name, emptyToNil(req.Description))
	if err != nil {
		if isUnique(err) {
			httpx.Error(w, http.StatusConflict, "Template name already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	for _, rule := range req.Rules {
		if msg := ValidateActions(rule.Actions); msg != "" {
			httpx.Error(w, http.StatusBadRequest, msg)
			return
		}
		bid, ok := h.resolveBucket(w, r, user, rule.BucketName)
		if !ok {
			return
		}
		if _, err := h.store.InsertTemplateRule(r.Context(), t.ID, bid, emptyToNil(rule.Prefix), rule.Actions); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
	}
	loaded, err := h.store.GetTemplate(r.Context(), accountID, t.ID)
	if err != nil || loaded == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusCreated, toTemplateJSON(*loaded))
}

func (h *Handler) getTemplate(w http.ResponseWriter, r *http.Request, user *auth.User) {
	_, t, ok := h.loadTemplate(w, r, user)
	if !ok {
		return
	}
	httpx.JSON(w, http.StatusOK, toTemplateJSON(*t))
}

func (h *Handler) updateTemplate(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, t, ok := h.loadTemplate(w, r, user)
	if !ok {
		return
	}
	var req templateUpdateReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.store.UpdateTemplate(r.Context(), accountID, t.ID, req.Name, req.Description); err != nil {
		if isUnique(err) {
			httpx.Error(w, http.StatusConflict, "Template name already exists")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	loaded, err := h.store.GetTemplate(r.Context(), accountID, t.ID)
	if err != nil || loaded == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, toTemplateJSON(*loaded))
}

func (h *Handler) deleteTemplate(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, t, ok := h.loadTemplate(w, r, user)
	if !ok {
		return
	}
	item := toTemplateJSON(*t)
	if err := h.store.DeleteTemplate(r.Context(), accountID, t.ID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handler) createTemplateRule(w http.ResponseWriter, r *http.Request, user *auth.User) {
	_, t, ok := h.loadTemplate(w, r, user)
	if !ok {
		return
	}
	var req ruleInput
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if msg := ValidateActions(req.Actions); msg != "" {
		httpx.Error(w, http.StatusBadRequest, msg)
		return
	}
	bid, ok := h.resolveBucket(w, r, user, req.BucketName)
	if !ok {
		return
	}
	rule, err := h.store.InsertTemplateRule(r.Context(), t.ID, bid, emptyToNil(req.Prefix), req.Actions)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusCreated, toRuleJSON(*rule))
}

func (h *Handler) updateTemplateRule(w http.ResponseWriter, r *http.Request, user *auth.User) {
	_, t, ok := h.loadTemplate(w, r, user)
	if !ok {
		return
	}
	ruleID, ok := pathID(r, "rule_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid rule_id")
		return
	}
	var req permUpdateReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	cur, err := h.store.getTemplateRule(r.Context(), t.ID, ruleID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if cur == nil {
		httpx.Error(w, http.StatusNotFound, "Template rule not found")
		return
	}
	bucketID, prefix, actions := cur.BucketID, cur.Prefix, cur.Actions
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
	if err := h.store.UpdateTemplateRule(r.Context(), t.ID, ruleID, bucketID, prefix, actions); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	updated, err := h.store.getTemplateRule(r.Context(), t.ID, ruleID)
	if err != nil || updated == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, toRuleJSON(*updated))
}

func (h *Handler) deleteTemplateRule(w http.ResponseWriter, r *http.Request, user *auth.User) {
	_, t, ok := h.loadTemplate(w, r, user)
	if !ok {
		return
	}
	ruleID, ok := pathID(r, "rule_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid rule_id")
		return
	}
	cur, err := h.store.getTemplateRule(r.Context(), t.ID, ruleID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if cur == nil {
		httpx.Error(w, http.StatusNotFound, "Template rule not found")
		return
	}
	item := toRuleJSON(*cur)
	if err := h.store.DeleteTemplateRule(r.Context(), t.ID, ruleID); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handler) createAssignment(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, t, ok := h.loadTemplate(w, r, user)
	if !ok {
		return
	}
	var req assignmentReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if (req.UserID == nil) == (req.GroupID == nil) {
		httpx.Error(w, http.StatusBadRequest, "Specify exactly one of user_id or group_id")
		return
	}
	if req.UserID != nil {
		u, err := h.users.GetInAccount(r.Context(), accountID, *req.UserID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if u == nil {
			httpx.Error(w, http.StatusBadRequest, "User not found")
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
			httpx.Error(w, http.StatusBadRequest, "User group not found for account")
			return
		}
	}
	a, err := h.store.InsertAssignment(r.Context(), accountID, t.ID, req.UserID, req.GroupID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusCreated, toAssignmentJSON(*a))
}

func (h *Handler) deleteAssignment(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, t, ok := h.loadTemplate(w, r, user)
	if !ok {
		return
	}
	id, ok := pathID(r, "assignment_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid assignment_id")
		return
	}
	cur, err := h.store.GetAssignment(r.Context(), accountID, t.ID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if cur == nil {
		httpx.Error(w, http.StatusNotFound, "Template assignment not found")
		return
	}
	item := toAssignmentJSON(*cur)
	if err := h.store.DeleteAssignment(r.Context(), accountID, t.ID, id); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, item)
}

func (h *Handler) loadTemplate(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, *Template, bool) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return 0, nil, false
	}
	id, ok := pathID(r, "template_id")
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "invalid template_id")
		return 0, nil, false
	}
	t, err := h.store.GetTemplate(r.Context(), accountID, id)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return 0, nil, false
	}
	if t == nil {
		httpx.Error(w, http.StatusNotFound, "Permission template not found")
		return 0, nil, false
	}
	return accountID, t, true
}

func toTemplateJSON(t Template) templateJSON {
	rules := make([]ruleJSON, 0, len(t.Rules))
	for _, r := range t.Rules {
		rules = append(rules, toRuleJSON(r))
	}
	as := make([]assignmentJSON, 0, len(t.Assignments))
	for _, a := range t.Assignments {
		as = append(as, toAssignmentJSON(a))
	}
	return templateJSON{
		ID: t.ID, TenantID: t.AccountID, Name: t.Name, Description: t.Description,
		Rules: rules, Assignments: as, CreatedAt: rfc3339(t.CreatedAt), UpdatedAt: rfc3339(t.UpdatedAt),
	}
}

func toRuleJSON(r TemplateRule) ruleJSON {
	return ruleJSON{
		ID: r.ID, TemplateID: r.TemplateID, BucketID: r.BucketID, BucketName: r.BucketName,
		Prefix: r.Prefix, Actions: r.Actions, CreatedAt: rfc3339(r.CreatedAt), UpdatedAt: rfc3339(r.UpdatedAt),
	}
}

func toAssignmentJSON(a Assignment) assignmentJSON {
	return assignmentJSON{ID: a.ID, TenantID: a.AccountID, TemplateID: a.TemplateID, UserID: a.UserID, GroupID: a.GroupID}
}
