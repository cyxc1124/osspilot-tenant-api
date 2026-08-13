package stats

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type InternalHandler struct {
	usage  *UsageStore
	secret string
}

func NewInternalHandler(usage *UsageStore, secret string) *InternalHandler {
	return &InternalHandler{usage: usage, secret: secret}
}

func (h *InternalHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /internal/stats/usage", h.usageHandler)
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

func bearerOK(header, secret string) bool {
	const p = "Bearer "
	if secret == "" || !strings.HasPrefix(header, p) {
		return false
	}
	got := strings.TrimSpace(header[len(p):])
	return subtle.ConstantTimeCompare([]byte(got), []byte(secret)) == 1
}
