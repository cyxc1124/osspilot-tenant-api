package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/config"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

func newMux(h *auth.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	if h != nil {
		h.Register(mux)
	}
	return httpx.CORS(mux)
}

func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func main() {
	cfg := config.Load()
	if cfg.DefaultJWTUsed {
		slog.Warn("JWT_SECRET unset; using development default")
	}

	var store *auth.Store
	if cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			slog.Error("db pool", "err", err)
			os.Exit(1)
		}
		defer pool.Close()
		store = auth.NewStore(pool)
	} else {
		slog.Warn("DATABASE_URL unset; auth routes return 503")
	}

	h := auth.NewHandler(store, cfg.JWTSecret, cfg.TokenTTL)
	addr := cfg.HTTPAddr
	slog.Info("listen", "addr", addr)
	if err := http.ListenAndServe(addr, newMux(h)); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
