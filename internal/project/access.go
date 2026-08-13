package project

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/creds"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/queue"
)

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("PUT /internal/accounts/{username}", h.upsert)
	mux.HandleFunc("DELETE /internal/accounts/{username}", h.remove)
	mux.HandleFunc("PUT /internal/accounts/{username}/buckets", h.replaceBuckets)
	mux.HandleFunc("GET /internal/api-access", h.listAccess)
	mux.HandleFunc("GET /internal/api-access/{username}", h.getAccess)
	mux.HandleFunc("POST /internal/api-access/{username}/approve", h.review("approve"))
	mux.HandleFunc("POST /internal/api-access/{username}/reject", h.review("reject"))
	mux.HandleFunc("POST /internal/api-access/{username}/disable", h.review("disable"))
	mux.HandleFunc("POST /internal/buckets/{bucket_name}/inventory", h.enqueueInventory)
}

func (h *Handler) enqueueInventory(w http.ResponseWriter, r *http.Request) {
	if h.secret == "" || h.buckets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return
	}
	if !bearerOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	if h.queue == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "task queue unavailable")
		return
	}
	name := strings.TrimSpace(r.PathValue("bucket_name"))
	if name == "" {
		httpx.Error(w, http.StatusBadRequest, "bucket_name is required")
		return
	}
	if _, err := h.buckets.Ensure(r.Context(), name, nil); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	jobID, err := h.queue.EnqueueInventory(r.Context(), name)
	if err != nil {
		if errors.Is(err, queue.ErrUnavailable) {
			httpx.Error(w, http.StatusServiceUnavailable, "task queue unavailable")
			return
		}
		httpx.Error(w, http.StatusInternalServerError, "queue error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"bucket_name": name, "job_id": jobID,
		"message": "容量统计任务已提交，请稍后刷新查看结果",
	})
}

func (h *Handler) gateAccess(w http.ResponseWriter, r *http.Request) bool {
	if h.secret == "" || h.users == nil || h.creds == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return false
	}
	if !bearerOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return false
	}
	return true
}

func (h *Handler) listAccess(w http.ResponseWriter, r *http.Request) {
	if !h.gateAccess(w, r) {
		return
	}
	items, err := h.creds.ListAccess(r.Context(), r.URL.Query().Get("status"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, accessJSON(item))
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "total": len(out)})
}

func (h *Handler) getAccess(w http.ResponseWriter, r *http.Request) {
	if !h.gateAccess(w, r) {
		return
	}
	item, err := h.creds.GetAccessByUsername(r.Context(), r.PathValue("username"))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if item == nil {
		httpx.Error(w, http.StatusNotFound, "Tenant API access record not found")
		return
	}
	httpx.JSON(w, http.StatusOK, accessJSON(*item))
}

func (h *Handler) review(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.gateAccess(w, r) {
			return
		}
		var req struct {
			Note *string `json:"note"`
		}
		if r.ContentLength != 0 {
			if err := httpx.DecodeJSON(r, &req); err != nil {
				httpx.Error(w, http.StatusBadRequest, "invalid json")
				return
			}
		}
		user, err := h.users.GetByUsername(r.Context(), r.PathValue("username"))
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if user == nil {
			httpx.Error(w, http.StatusNotFound, "Account not found")
			return
		}
		var uid *string
		if action == "approve" {
			s := fmt.Sprintf("account-%d-%s", user.ID, user.Username)
			uid = &s
		}
		item, err := h.creds.ReviewAccess(r.Context(), user.ID, action, req.Note, uid)
		if err != nil {
			var ce *creds.ConflictError
			if errors.As(err, &ce) {
				httpx.Error(w, http.StatusConflict, ce.Msg)
				return
			}
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if item == nil {
			httpx.Error(w, http.StatusNotFound, "Tenant API access record not found")
			return
		}
		httpx.JSON(w, http.StatusOK, accessJSON(*item))
	}
}

func accessJSON(a creds.AccessRow) map[string]any {
	return map[string]any{
		"id": a.ID, "account_id": a.AccountID, "account_name": a.Username, "status": a.Status,
		"requested_by": a.RequestedBy, "requested_at": rfc3339p(a.RequestedAt),
		"reviewed_by": nil, "reviewed_at": rfc3339p(a.ReviewedAt), "review_note": a.ReviewNote,
		"rgw_uid": a.RGWUID, "application_count": a.AppCount, "access_key_count": a.KeyCount,
	}
}

func rfc3339p(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}
