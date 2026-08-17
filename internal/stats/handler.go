package stats

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
)

type Handler struct {
	store   *Store
	buckets *bucket.Store
	users   *auth.Store
	protect func(auth.UserHandler) http.HandlerFunc
	ac      *rbac.Checker
}

func NewHandler(store *Store, buckets *bucket.Store, users *auth.Store, protect func(auth.UserHandler) http.HandlerFunc, ac *rbac.Checker) *Handler {
	return &Handler{store: store, buckets: buckets, users: users, protect: protect, ac: ac}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/stats", h.protect(h.account))
	mux.HandleFunc("GET /api/stats/buckets", h.protect(h.bucketStats))
	mux.HandleFunc("GET /api/stats/traffic", h.protect(h.traffic))
	mux.HandleFunc("GET /api/stats/buckets/requests", h.protect(h.bucketRequests))
	mux.HandleFunc("GET /api/stats/prefixes", h.protect(h.prefixes))
	mux.HandleFunc("GET /api/alerts/notifications", h.protect(h.alerts))
}

func (h *Handler) accountID(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, bool) {
	if h.buckets == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "database is not configured")
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

func (h *Handler) gate(w http.ResponseWriter, r *http.Request, user *auth.User) (int64, bool) {
	id, ok := h.accountID(w, r, user)
	if !ok {
		return 0, false
	}
	if h.ac.Forbidden(w, r, user, "", "", rbac.ActionRead) {
		return 0, false
	}
	return id, true
}

func (h *Handler) visible(w http.ResponseWriter, r *http.Request, user *auth.User, accountID int64) ([]bucket.Bucket, bool) {
	items, err := h.buckets.List(r.Context(), accountID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return nil, false
	}
	if auth.IsAdmin(user) || h.ac == nil {
		return items, true
	}
	out := make([]bucket.Bucket, 0, len(items))
	for _, b := range items {
		ok, err := h.ac.Allow(r.Context(), user, b.BucketName, "", rbac.ActionRead)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return nil, false
		}
		if ok {
			out = append(out, b)
		}
	}
	return out, true
}

func (h *Handler) account(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	items, ok := h.visible(w, r, user, accountID)
	if !ok {
		return
	}
	ids := idsOf(items)
	usage, err := h.buckets.UsageByID(r.Context(), ids)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	var used, count, trashB, trashN, verB, verN int64
	for _, u := range usage {
		used += u.UsedBytes
		count += u.ObjectCount
		trashB += u.TrashBytes
		trashN += u.TrashCount
		verB += u.VersionBytes
		verN += u.VersionCount
	}
	var collected any
	if len(items) > 0 {
		collected = time.Now().UTC().Format(time.RFC3339)
	}
	var quota *int64
	if h.users != nil {
		acc, err := h.users.GetByID(r.Context(), accountID)
		if err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		if acc != nil {
			quota = acc.QuotaBytes
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"tenant_id": accountID, "quota_bytes": quota, "used_bytes": used,
		"remaining_bytes": remainingBytes(quota, used), "object_count": count,
		"trash_bytes": trashB, "trash_object_count": trashN,
		"version_bytes": verB, "version_object_count": verN,
		"usage_percent": usagePercent(used, quota), "collected_at": collected,
	})
}

func (h *Handler) bucketStats(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	items, ok := h.visible(w, r, user, accountID)
	if !ok {
		return
	}
	usage, err := h.buckets.UsageByID(r.Context(), idsOf(items))
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	var collected any
	if len(items) > 0 {
		collected = time.Now().UTC().Format(time.RFC3339)
	}
	for _, b := range items {
		u := usage[b.ID]
		out = append(out, map[string]any{
			"bucket_id": b.ID, "bucket_name": b.BucketName, "display_name": b.DisplayName,
			"display_alias_only": b.DisplayAliasOnly, "quota_bytes": b.QuotaBytes,
			"used_bytes": u.UsedBytes, "object_count": u.ObjectCount,
			"trash_bytes": u.TrashBytes, "trash_object_count": u.TrashCount,
			"usage_percent": usagePercent(u.UsedBytes, b.QuotaBytes),
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out, "collected_at": collected})
}

func (h *Handler) traffic(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	period, okp := parsePeriod(r.URL.Query().Get("period"))
	if !okp {
		httpx.Error(w, http.StatusBadRequest, "period must be 24h, 7d, or 30d")
		return
	}
	row, err := h.store.AccountTraffic(r.Context(), accountID, period)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"tenant_id": accountID, "period": period,
		"upload_bytes": row.UploadBytes, "download_bytes": row.DownloadBytes, "request_count": row.RequestCount,
		"get_count": row.GetCount, "put_count": row.PutCount, "delete_count": row.DeleteCount,
		"error_count": row.ErrorCount, "active_users": row.ActiveUsers, "collected_at": rfc3339(row.CollectedAt),
	})
}

func (h *Handler) bucketRequests(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	period, okp := parsePeriod(r.URL.Query().Get("period"))
	if !okp {
		httpx.Error(w, http.StatusBadRequest, "period must be 24h, 7d, or 30d")
		return
	}
	items, ok := h.visible(w, r, user, accountID)
	if !ok {
		return
	}
	byID, err := h.store.BucketTraffic(r.Context(), idsOf(items), period)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	var collected *time.Time
	for _, b := range items {
		row := byID[b.ID]
		if row.CollectedAt != nil && (collected == nil || row.CollectedAt.After(*collected)) {
			collected = row.CollectedAt
		}
		out = append(out, map[string]any{
			"bucket_id": b.ID, "bucket_name": b.BucketName, "display_name": b.DisplayName,
			"display_alias_only": b.DisplayAliasOnly, "request_count": row.RequestCount,
			"upload_bytes": row.UploadBytes, "download_bytes": row.DownloadBytes,
			"get_count": row.GetCount, "put_count": row.PutCount, "delete_count": row.DeleteCount,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i]["request_count"].(int64) > out[j]["request_count"].(int64)
	})
	httpx.JSON(w, http.StatusOK, map[string]any{"tenant_id": accountID, "period": period, "items": out, "collected_at": rfc3339(collected)})
}

func (h *Handler) prefixes(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok {
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("bucket_name"))
	if name == "" {
		httpx.Error(w, http.StatusBadRequest, "bucket_name is required")
		return
	}
	items, ok := h.visible(w, r, user, accountID)
	if !ok {
		return
	}
	var b *bucket.Bucket
	for i := range items {
		if items[i].BucketName == name {
			b = &items[i]
			break
		}
	}
	if b == nil {
		httpx.Error(w, http.StatusNotFound, "Bucket not found")
		return
	}
	rows, err := h.buckets.PrefixUsage(r.Context(), b.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(rows))
	var collected any
	if len(rows) > 0 {
		collected = time.Now().UTC().Format(time.RFC3339)
	}
	for _, p := range rows {
		out = append(out, map[string]any{
			"prefix": displayPrefix(p.Prefix), "used_bytes": p.UsedBytes, "object_count": p.ObjectCount,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"bucket_name": b.BucketName, "items": out, "collected_at": collected})
}

func (h *Handler) alerts(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.accountID(w, r, user)
	if !ok {
		return
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			httpx.Error(w, http.StatusBadRequest, "limit must be 1-50")
			return
		}
		limit = n
	}
	if h.store == nil {
		httpx.JSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}
	items, err := h.store.ListAlerts(r.Context(), accountID, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, a := range items {
		var resolved any
		if a.ResolvedAt != nil {
			resolved = a.ResolvedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, map[string]any{
			"id": a.ID, "rule_type": a.RuleType, "severity": a.Severity, "status": a.Status,
			"title": a.Title, "message": a.Message, "bucket_name": a.BucketName,
			"fired_at": a.FiredAt.UTC().Format(time.RFC3339), "resolved_at": resolved,
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func idsOf(items []bucket.Bucket) []int64 {
	ids := make([]int64, 0, len(items))
	for _, b := range items {
		ids = append(ids, b.ID)
	}
	return ids
}
