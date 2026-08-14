package platform

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

// Keys ops may project into platform_settings (brand, CDN, cleanup).
var projectable = map[string]struct{}{
	"tenant_login_logo_text":    {},
	"tenant_login_title":        {},
	"tenant_login_subtitle":     {},
	"download_cdn_url":          {},
	"preview_cdn_url":           {},
	"object_http_domain":        {},
	"object_https_domain":       {},
	"trash_retention_days":      {},
	"trash_cleanup_enabled":     {},
	"version_retention_days":    {},
	"version_cleanup_enabled":   {},
	"multipart_stale_days":      {},
	"multipart_cleanup_enabled": {},
	"storage_region_id":         {},
	"storage_region_code":       {},
	"storage_region_name":       {},
	"s3_endpoint":               {},
}

func (h *Handler) registerInternal(mux *http.ServeMux) {
	mux.HandleFunc("PUT /internal/settings", h.putInternal)
}

func (h *Handler) putInternal(w http.ResponseWriter, r *http.Request) {
	if h.secret == "" || h.store == nil {
		httpx.Error(w, http.StatusServiceUnavailable, "projection is not configured")
		return
	}
	if !projectionOK(r.Header.Get("Authorization"), h.secret) {
		httpx.Error(w, http.StatusUnauthorized, "invalid token")
		return
	}
	var req struct {
		Settings map[string]string `json:"settings"`
	}
	if err := httpx.DecodeJSON(r, &req); err != nil || req.Settings == nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	n := 0
	for k, v := range req.Settings {
		if _, ok := projectable[k]; !ok {
			continue
		}
		if err := h.store.Upsert(r.Context(), k, v); err != nil {
			httpx.Error(w, http.StatusInternalServerError, "database error")
			return
		}
		n++
	}
	if n == 0 {
		httpx.Error(w, http.StatusBadRequest, "No projectable fields")
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
