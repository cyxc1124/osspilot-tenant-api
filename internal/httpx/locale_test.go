package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLocalizeZH(t *testing.T) {
	msg, code := Localize("Invalid username or password", "zh-CN")
	if msg != "用户名或密码错误" || code != "auth.invalid_credentials" {
		t.Fatalf("%q %q", msg, code)
	}
}

func TestErrorWritesLocale(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /e", func(w http.ResponseWriter, r *http.Request) {
		Error(w, http.StatusUnauthorized, "Invalid username or password")
	})
	srv := httptest.NewServer(CORS(mux))
	t.Cleanup(srv.Close)
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/e", nil)
	req.Header.Set("X-App-Locale", "zh-CN")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var body ErrorBody
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Detail != "用户名或密码错误" || body.Locale != "zh-CN" || body.ErrorCode != "auth.invalid_credentials" {
		t.Fatalf("%+v", body)
	}
}
