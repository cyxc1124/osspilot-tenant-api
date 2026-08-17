package openapidoc

import (
	_ "embed"
	"net/http"

	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

//go:embed openapi.yaml
var Spec []byte

func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1", root)
	mux.HandleFunc("GET /api/v1/{$}", root)
	mux.HandleFunc("GET /api/v1/openapi.json", spec)
	mux.HandleFunc("GET /api/v1/docs", docs)
}

func root(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{
		"api": "OssPilot Open API", "version": "v1", "scope": "api",
		"openapi": "/api/v1/openapi.json", "docs": "/api/v1/docs",
	})
}

func spec(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(Spec)
}

func docs(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html><html><head><title>OssPilot Open API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css"></head>
<body><div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
<script>SwaggerUIBundle({url:"/api/v1/openapi.json",dom_id:"#swagger-ui"})</script>
</body></html>`))
}
