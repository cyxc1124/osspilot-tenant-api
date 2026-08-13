package platform

import (
	"net/http"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type Fallbacks struct {
	S3Endpoint        string
	DownloadCDNURL    string
	PreviewCDNURL     string
	ObjectHTTPDomain  string
	ObjectHTTPSDomain string
}

type Region struct {
	ID         int64  `json:"id"`
	Code       string `json:"code"`
	Name       string `json:"name"`
	S3Endpoint string `json:"s3_endpoint"`
}

type configResponse struct {
	TenantID            int64   `json:"tenant_id"`
	StorageRegion       *Region `json:"storage_region"`
	S3Endpoint          *string `json:"s3_endpoint"`
	DownloadCDNURL      *string `json:"download_cdn_url"`
	PreviewCDNURL       *string `json:"preview_cdn_url"`
	ObjectHTTPDomain    *string `json:"object_http_domain"`
	ObjectHTTPSDomain   *string `json:"object_https_domain"`
	TrashRetentionDays  int     `json:"trash_retention_days"`
	TrashCleanupEnabled bool    `json:"trash_cleanup_enabled"`
}

type Handler struct {
	store     *Store
	fallbacks Fallbacks
	protect   func(auth.UserHandler) http.HandlerFunc
}

func NewHandler(store *Store, protect func(auth.UserHandler) http.HandlerFunc, fallbacks Fallbacks) *Handler {
	return &Handler{store: store, protect: protect, fallbacks: fallbacks}
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/login-branding", h.branding)
	mux.HandleFunc("GET /api/platform-config", h.protect(h.config))
}

func (h *Handler) branding(w http.ResponseWriter, r *http.Request) {
	rows, err := h.rows(r)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, brandingFrom(rows))
}

func (h *Handler) config(w http.ResponseWriter, r *http.Request, user *auth.User) {
	rows, err := h.rows(r)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	httpx.JSON(w, http.StatusOK, configResponse{
		TenantID:            user.ID,
		StorageRegion:       nil, // ponytail: until O3/O4 projects regions
		S3Endpoint:          pickPtr(rows, "s3_endpoint", h.fallbacks.S3Endpoint),
		DownloadCDNURL:      pickPtr(rows, "download_cdn_url", h.fallbacks.DownloadCDNURL),
		PreviewCDNURL:       pickPtr(rows, "preview_cdn_url", h.fallbacks.PreviewCDNURL),
		ObjectHTTPDomain:    pickPtr(rows, "object_http_domain", h.fallbacks.ObjectHTTPDomain),
		ObjectHTTPSDomain:   pickPtr(rows, "object_https_domain", h.fallbacks.ObjectHTTPSDomain),
		TrashRetentionDays:  pickInt(rows, "trash_retention_days", 0),
		TrashCleanupEnabled: pickBool(rows, "trash_cleanup_enabled", false),
	})
}

func (h *Handler) rows(r *http.Request) (map[string]string, error) {
	if h.store == nil {
		return map[string]string{}, nil
	}
	return h.store.Map(r.Context())
}
