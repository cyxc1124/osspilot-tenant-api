package stats

import (
	"crypto/subtle"
	"net/http"
	"strconv"
	"strings"

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
		})
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"used_bytes": totals.UsedBytes, "object_count": totals.ObjectCount,
		"trash_bytes": totals.TrashBytes, "trash_object_count": totals.TrashCount,
		"version_bytes": totals.VersionBytes, "version_object_count": totals.VersionCount,
		"buckets": items,
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

func bearerOK(header, secret string) bool {
	const p = "Bearer "
	if secret == "" || !strings.HasPrefix(header, p) {
		return false
	}
	got := strings.TrimSpace(header[len(p):])
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}
