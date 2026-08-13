package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
)

func newMux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	return mux
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func listenAddr() string {
	if v := os.Getenv("HTTP_ADDR"); v != "" {
		return v
	}
	return ":8000"
}

func main() {
	addr := listenAddr()
	slog.Info("listen", "addr", addr)
	if err := http.ListenAndServe(addr, newMux()); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
