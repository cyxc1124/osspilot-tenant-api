package openapidoc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAPIDocs(t *testing.T) {
	if !strings.Contains(string(Spec), "/api/v1/buckets") {
		t.Fatal("embedded spec missing /api/v1/buckets")
	}
	mux := http.NewServeMux()
	Register(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/api/v1/openapi.json") {
		t.Fatalf("root %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "openapi:") {
		t.Fatalf("spec %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "swagger-ui") {
		t.Fatalf("docs %d", rec.Code)
	}
}
