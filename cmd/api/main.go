package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/config"
	"github.com/cyxc1124/osspilot-tenant-api/internal/downloads"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/platform"
	"github.com/cyxc1124/osspilot-tenant-api/internal/project"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
	"github.com/cyxc1124/osspilot-tenant-api/internal/uploads"
)

func newMux(authH *auth.Handler, bucketH *bucket.Handler, objectH *objects.Handler, uploadH *uploads.Handler, downloadH *downloads.Handler, platformH *platform.Handler, projectH *project.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	if authH != nil {
		authH.Register(mux)
	}
	if bucketH != nil {
		bucketH.Register(mux)
	}
	if objectH != nil {
		objectH.Register(mux)
	}
	if uploadH != nil {
		uploadH.Register(mux)
	}
	if downloadH != nil {
		downloadH.Register(mux)
	}
	if platformH != nil {
		platformH.Register(mux)
	}
	if projectH != nil {
		projectH.Register(mux)
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

	var authStore *auth.Store
	var bucketStore *bucket.Store
	var objectStore *objects.Store
	var uploadStore *uploads.Store
	var settingsStore *platform.Store
	if cfg.DatabaseURL != "" {
		pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
		if err != nil {
			slog.Error("db pool", "err", err)
			os.Exit(1)
		}
		defer pool.Close()
		authStore = auth.NewStore(pool)
		bucketStore = bucket.NewStore(pool)
		objectStore = objects.NewStore(pool)
		uploadStore = uploads.NewStore(pool)
		settingsStore = platform.NewStore(pool)
	} else {
		slog.Warn("DATABASE_URL unset; auth routes return 503")
	}

	s3cfg := storage.Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.RGWAccessKey,
		SecretKey: cfg.RGWSecretKey,
		Region:    cfg.S3Region,
	}
	var s3c *storage.Client
	if s3cfg.Ready() {
		s3c = storage.New(s3cfg)
	} else {
		slog.Warn("S3_ENDPOINT/RGW_ACCESS_KEY/RGW_SECRET_KEY unset; upload/download return 503")
	}

	authH := auth.NewHandler(authStore, cfg.JWTSecret, cfg.TokenTTL)
	bucketH := bucket.NewHandler(bucketStore, authH.RequireUser, s3c, cfg.CORSOrigins)
	objectH := objects.NewHandler(bucketStore, objectStore, s3c, authH.RequireUser)
	uploadH := uploads.NewHandler(s3c, bucketStore, objectStore, uploadStore, authH.RequireUser)
	downloadH := downloads.NewHandler(s3c, bucketStore, authH.RequireUser)
	platformH := platform.NewHandler(settingsStore, authH.RequireUser, platform.Fallbacks{
		S3Endpoint:        cfg.S3Endpoint,
		DownloadCDNURL:    cfg.DownloadCDNURL,
		PreviewCDNURL:     cfg.PreviewCDNURL,
		ObjectHTTPDomain:  cfg.ObjectHTTPDomain,
		ObjectHTTPSDomain: cfg.ObjectHTTPSDomain,
	})
	projectH := project.NewHandler(cfg.ProjectionSecret, authStore, bucketStore)
	if cfg.ProjectionSecret == "" {
		slog.Warn("PROJECTION_SECRET unset; internal projection routes return 503")
	}
	addr := cfg.HTTPAddr
	slog.Info("listen", "addr", addr)
	if err := http.ListenAndServe(addr, newMux(authH, bucketH, objectH, uploadH, downloadH, platformH, projectH)); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
