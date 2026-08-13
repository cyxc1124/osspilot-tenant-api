package bucket

import (
	"net/http"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
)

func (h *Handler) getPolicy(w http.ResponseWriter, r *http.Request, user *auth.User) {
	b, ok := h.load(w, r, user, rbac.ActionAdmin)
	if !ok {
		return
	}
	if !h.requireS3(w) {
		return
	}
	policy, err := h.s3.GetBucketPolicy(r.Context(), b.BucketName)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"bucket_name": b.BucketName,
		"policy":      policy,
		"has_policy":  policy != nil,
	})
}

func (h *Handler) putPolicy(w http.ResponseWriter, r *http.Request, user *auth.User) {
	b, ok := h.load(w, r, user, rbac.ActionAdmin)
	if !ok {
		return
	}
	if !h.requireS3(w) {
		return
	}
	var req struct {
		Policy map[string]any `json:"policy"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := validatePolicy(req.Policy); err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.s3.PutBucketPolicy(r.Context(), b.BucketName, req.Policy); err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"bucket_name": b.BucketName,
		"policy":      req.Policy,
		"has_policy":  true,
	})
}

func (h *Handler) deletePolicy(w http.ResponseWriter, r *http.Request, user *auth.User) {
	b, ok := h.load(w, r, user, rbac.ActionAdmin)
	if !ok {
		return
	}
	if !h.requireS3(w) {
		return
	}
	if err := h.s3.DeleteBucketPolicy(r.Context(), b.BucketName); err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) getCors(w http.ResponseWriter, r *http.Request, user *auth.User) {
	b, ok := h.load(w, r, user, rbac.ActionAdmin)
	if !ok {
		return
	}
	if !h.requireS3(w) {
		return
	}
	rules, err := h.s3.GetBucketCORS(r.Context(), b.BucketName)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	out := fromStorageCORS(rules)
	if out == nil {
		out = []corsRule{}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"bucket_name": b.BucketName,
		"cors_rules":  out,
		"has_cors":    len(out) > 0,
	})
}

func (h *Handler) putCors(w http.ResponseWriter, r *http.Request, user *auth.User) {
	b, ok := h.load(w, r, user, rbac.ActionAdmin)
	if !ok {
		return
	}
	if !h.requireS3(w) {
		return
	}
	var req struct {
		CorsRules []corsRule `json:"cors_rules"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	validated, err := validateCorsRules(req.CorsRules)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.s3.PutBucketCORS(r.Context(), b.BucketName, toStorageCORS(validated)); err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"bucket_name": b.BucketName,
		"cors_rules":  responseCors(validated),
		"has_cors":    true,
	})
}

func (h *Handler) deleteCors(w http.ResponseWriter, r *http.Request, user *auth.User) {
	b, ok := h.load(w, r, user, rbac.ActionAdmin)
	if !ok {
		return
	}
	if !h.requireS3(w) {
		return
	}
	if err := h.s3.DeleteBucketCORS(r.Context(), b.BucketName); err != nil {
		httpx.Error(w, http.StatusBadGateway, "storage error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) requireS3(w http.ResponseWriter) bool {
	if h.s3 == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "storage is not configured")
		return false
	}
	return true
}

func responseCors(rules []corsRule) []corsRule {
	out := make([]corsRule, len(rules))
	for i, r := range rules {
		out[i] = r
		if out[i].AllowedHeaders == nil {
			out[i].AllowedHeaders = []string{"*"}
		}
		if out[i].ExposeHeaders == nil {
			out[i].ExposeHeaders = []string{}
		}
	}
	return out
}
