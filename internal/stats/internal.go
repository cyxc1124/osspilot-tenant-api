package stats

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type InternalHandler struct {
	usage  *UsageStore
	reqs   *Store
	secret string
}

func NewInternalHandler(usage *UsageStore, reqs *Store, secret string) *InternalHandler {
	return &InternalHandler{usage: usage, reqs: reqs, secret: secret}
}

func (h *InternalHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /internal/stats/usage", h.usageHandler)
	mux.HandleFunc("GET /internal/stats/requests", h.requestsHandler)
	mux.HandleFunc("PUT /internal/alerts/events", h.putAlert)
	mux.HandleFunc("GET /internal/stats/audit-window", h.auditWindow)
}

func (h *InternalHandler) usageHandler(w http.ResponseWriter, r *http.Request) {
	if h.secret == "" || h.usage == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return
	}
	if !bearerOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	totals, err := h.usage.Totals(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	buckets, err := h.usage.ByBucket(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	items := make([]map[string]any, 0, len(buckets))
	for _, b := range buckets {
		items = append(items, map[string]any{
			"bucket_name": b.BucketName, "used_bytes": b.UsedBytes, "object_count": b.ObjectCount,
			"trash_bytes": b.TrashBytes, "trash_object_count": b.TrashCount,
			"collected_at": rfc3339(b.CollectedAt),
		})
	}
	classes, collected, err := h.usage.ByStorageClass(r.Context())
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	sc := make([]map[string]any, 0, len(classes))
	for _, c := range classes {
		sc = append(sc, map[string]any{"storage_class": c.StorageClass, "used_bytes": c.UsedBytes})
	}
	var collectedAt any
	if collected != nil {
		collectedAt = collected.UTC().Format(time.RFC3339)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"used_bytes": totals.UsedBytes, "object_count": totals.ObjectCount,
		"trash_bytes": totals.TrashBytes, "trash_object_count": totals.TrashCount,
		"version_bytes": totals.VersionBytes, "version_object_count": totals.VersionCount,
		"buckets": items, "storage_classes": sc, "collected_at": collectedAt,
	})
}

func (h *InternalHandler) requestsHandler(w http.ResponseWriter, r *http.Request) {
	if h.secret == "" || h.reqs == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return
	}
	if !bearerOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	period, ok := parsePeriod(r.URL.Query().Get("period"))
	if !ok {
		httpx.Error(w, http.StatusBadRequest, "period must be 24h, 7d, or 30d")
		return
	}
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 90 {
			httpx.Error(w, http.StatusBadRequest, "days must be 1-90")
			return
		}
		days = n
	}
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			httpx.Error(w, http.StatusBadRequest, "limit must be 1-100")
			return
		}
		limit = n
	}
	sortBy := r.URL.Query().Get("sort_by")
	plat, err := h.reqs.PlatformTraffic(r.Context(), period)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	daily, err := h.reqs.DailyTraffic(r.Context(), days)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	users, err := h.reqs.UserRanking(r.Context(), period, sortBy, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	buckets, err := h.reqs.BucketRanking(r.Context(), period, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	prefixes, err := h.reqs.PrefixRanking(r.Context(), period, limit)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	ditems := make([]map[string]any, 0, len(daily))
	var dailyCollected any
	for _, d := range daily {
		ditems = append(ditems, map[string]any{
			"stat_date":    d.StatDate.UTC().Format("2006-01-02"),
			"upload_bytes": d.UploadBytes, "download_bytes": d.DownloadBytes, "request_count": d.RequestCount,
			"get_count": d.GetCount, "put_count": d.PutCount, "delete_count": d.DeleteCount, "error_count": d.ErrorCount,
		})
		t := d.CollectedAt
		dailyCollected = rfc3339(&t)
	}
	uitems := make([]map[string]any, 0, len(users))
	var userCollected any
	for _, u := range users {
		uitems = append(uitems, map[string]any{
			"user_id": u.UserID, "tenant_id": u.AccountID, "username": u.Username,
			"upload_count": u.UploadCount, "download_count": u.DownloadCount, "delete_count": u.DeleteCount,
			"access_count": u.AccessCount, "upload_bytes": u.UploadBytes, "download_bytes": u.DownloadBytes,
		})
		t := u.CollectedAt
		userCollected = rfc3339(&t)
	}
	bitems := make([]map[string]any, 0, len(buckets))
	var bktCollected any
	for _, b := range buckets {
		var tenant any
		if b.HasAccount {
			tenant = b.AccountID
		}
		bitems = append(bitems, map[string]any{
			"bucket_id": b.BucketID, "tenant_id": tenant, "bucket_name": b.BucketName,
			"request_count": b.RequestCount, "upload_bytes": b.UploadBytes, "download_bytes": b.DownloadBytes,
			"get_count": b.GetCount, "put_count": b.PutCount, "delete_count": b.DeleteCount,
		})
		t := b.CollectedAt
		bktCollected = rfc3339(&t)
	}
	pitems := make([]map[string]any, 0, len(prefixes))
	var preCollected any
	for _, p := range prefixes {
		var tenant any
		if p.HasAccount {
			tenant = p.AccountID
		}
		prefix := p.Prefix
		if prefix == "" {
			prefix = "(root)"
		}
		pitems = append(pitems, map[string]any{
			"bucket_id": p.BucketID, "tenant_id": tenant, "bucket_name": p.BucketName,
			"prefix": prefix, "access_count": p.AccessCount,
		})
		t := p.CollectedAt
		preCollected = rfc3339(&t)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"period": period,
		"platform": map[string]any{
			"period": period, "upload_bytes": plat.UploadBytes, "download_bytes": plat.DownloadBytes,
			"request_count": plat.RequestCount, "get_count": plat.GetCount, "put_count": plat.PutCount,
			"delete_count": plat.DeleteCount, "error_count": plat.ErrorCount, "active_users": plat.ActiveUsers,
			"collected_at": rfc3339(plat.CollectedAt),
		},
		"daily":    map[string]any{"items": ditems, "collected_at": dailyCollected},
		"users":    map[string]any{"items": uitems, "collected_at": userCollected},
		"buckets":  map[string]any{"items": bitems, "collected_at": bktCollected},
		"prefixes": map[string]any{"items": pitems, "collected_at": preCollected},
	})
}

func (h *InternalHandler) putAlert(w http.ResponseWriter, r *http.Request) {
	if h.secret == "" || h.reqs == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return
	}
	if !bearerOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var req struct {
		Username    string  `json:"username"`
		Fingerprint string  `json:"fingerprint"`
		RuleType    string  `json:"rule_type"`
		Severity    string  `json:"severity"`
		Status      string  `json:"status"`
		Title       string  `json:"title"`
		Message     string  `json:"message"`
		BucketName  *string `json:"bucket_name"`
		FiredAt     *string `json:"fired_at"`
		Resolved    bool    `json:"resolved"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Fingerprint == "" || req.Username == "" {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.Resolved || req.Status == "resolved" {
		if err := h.reqs.ResolveAlert(r.Context(), req.Fingerprint); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	u, err := h.reqs.AccountIDByUsername(r.Context(), req.Username)
	if err != nil || u == 0 {
		httpx.Error(w, http.StatusNotFound, "Account not found")
		return
	}
	a := Alert{RuleType: req.RuleType, Severity: req.Severity, Status: "firing", Title: req.Title, Message: req.Message, BucketName: req.BucketName, Fingerprint: req.Fingerprint}
	if req.Status != "" {
		a.Status = req.Status
	}
	if req.FiredAt != nil {
		if t, err := time.Parse(time.RFC3339, *req.FiredAt); err == nil {
			a.FiredAt = t
		}
	}
	if err := h.reqs.UpsertAlert(r.Context(), u, a); err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *InternalHandler) auditWindow(w http.ResponseWriter, r *http.Request) {
	if h.secret == "" || h.reqs == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return
	}
	if !bearerOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	minutes := 60
	if raw := r.URL.Query().Get("minutes"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 10080 {
			httpx.Error(w, http.StatusBadRequest, "minutes must be 1-10080")
			return
		}
		minutes = n
	}
	actions := splitCSV(r.URL.Query().Get("actions"))
	if len(actions) == 0 {
		httpx.Error(w, http.StatusBadRequest, "actions is required")
		return
	}
	total, fail, err := h.reqs.AuditWindow(r.Context(), time.Now().Add(-time.Duration(minutes)*time.Minute), actions)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"total": total, "failures": fail})
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func bearerOK(header, secret string) bool {
	const p = "Bearer "
	if secret == "" || !strings.HasPrefix(header, p) {
		return false
	}
	got := strings.TrimSpace(header[len(p):])
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}
