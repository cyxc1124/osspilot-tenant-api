package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyxc1124/osspilot-tenant-api/internal/audit"
	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/buildinfo"
	"github.com/cyxc1124/osspilot-tenant-api/internal/config"
	"github.com/cyxc1124/osspilot-tenant-api/internal/creds"
	"github.com/cyxc1124/osspilot-tenant-api/internal/downloads"
	"github.com/cyxc1124/osspilot-tenant-api/internal/edit"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/platform"
	"github.com/cyxc1124/osspilot-tenant-api/internal/preview"
	"github.com/cyxc1124/osspilot-tenant-api/internal/project"
	"github.com/cyxc1124/osspilot-tenant-api/internal/queue"
	"github.com/cyxc1124/osspilot-tenant-api/internal/quota"
	"github.com/cyxc1124/osspilot-tenant-api/internal/rbac"
	"github.com/cyxc1124/osspilot-tenant-api/internal/share"
	"github.com/cyxc1124/osspilot-tenant-api/internal/stats"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
	"github.com/cyxc1124/osspilot-tenant-api/internal/uploads"
	"github.com/cyxc1124/osspilot-tenant-api/internal/versions"
)

type apiHandlers struct {
	auth      *auth.Handler
	bucket    *bucket.Handler
	objects   *objects.Handler
	uploads   *uploads.Handler
	downloads *downloads.Handler
	platform  *platform.Handler
	project   *project.Handler
	versions  *versions.Handler
	share     *share.Handler
	edit      *edit.Handler
	rbac      *rbac.Handler
	creds     *creds.Handler
	stats     *stats.Handler
	audit     *audit.Handler
	preview   *preview.Handler
	usage     *stats.InternalHandler
}

func newMux(h apiHandlers) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", buildinfo.Healthz)
	if h.auth != nil {
		h.auth.Register(mux)
	}
	if h.bucket != nil {
		h.bucket.Register(mux)
	}
	if h.objects != nil {
		h.objects.Register(mux)
	}
	if h.uploads != nil {
		h.uploads.Register(mux)
	}
	if h.downloads != nil {
		h.downloads.Register(mux)
	}
	if h.platform != nil {
		h.platform.Register(mux)
	}
	if h.project != nil {
		h.project.Register(mux)
	}
	if h.versions != nil {
		h.versions.Register(mux)
	}
	if h.share != nil {
		h.share.Register(mux)
	}
	if h.edit != nil {
		h.edit.Register(mux)
	}
	if h.rbac != nil {
		h.rbac.Register(mux)
	}
	if h.creds != nil {
		h.creds.Register(mux)
	}
	if h.stats != nil {
		h.stats.Register(mux)
	}
	if h.audit != nil {
		h.audit.Register(mux)
	}
	if h.preview != nil {
		h.preview.Register(mux)
	}
	if h.usage != nil {
		h.usage.Register(mux)
	}
	return httpx.CORS(mux)
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
	var versionStore *versions.Store
	var shareStore *share.Store
	var editStore *edit.Store
	var rbacStore *rbac.Store
	var credsStore *creds.Store
	var statsStore *stats.Store
	var auditStore *audit.Store
	var auditLog *audit.Logger
	var pool *pgxpool.Pool
	if cfg.DatabaseURL != "" {
		var err error
		pool, err = pgxpool.New(context.Background(), cfg.DatabaseURL)
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
		versionStore = versions.NewStore(pool)
		shareStore = share.NewStore(pool)
		editStore = edit.NewStore(pool)
		rbacStore = rbac.NewStore(pool)
		credsStore = creds.NewStore(pool)
		statsStore = stats.NewStore(pool)
		auditStore = audit.NewStore(pool)
		auditLog = audit.NewLogger(pool)
	} else {
		slog.Warn("DATABASE_URL unset; auth routes return 503")
	}

	s3cfg := storage.Config{
		Endpoint: cfg.S3Endpoint, AccessKey: cfg.RGWAccessKey, SecretKey: cfg.RGWSecretKey, Region: cfg.S3Region,
		DownloadCDNURL: cfg.DownloadCDNURL, PreviewCDNURL: cfg.PreviewCDNURL,
	}
	if settingsStore != nil {
		if rows, err := settingsStore.Map(context.Background()); err == nil {
			s3cfg = storage.Overlay(s3cfg, rows)
		}
	}
	var s3c *storage.Client
	if s3cfg.Ready() {
		s3c = storage.New(s3cfg)
	} else {
		slog.Warn("S3_ENDPOINT/RGW_ACCESS_KEY/RGW_SECRET_KEY unset; upload/download return 503")
	}

	authH := auth.NewHandler(authStore, cfg.JWTSecret, cfg.TokenTTL)
	rbacH := rbac.NewHandler(authStore, rbacStore, bucketStore, authH.RequireUser, auditLog, cfg.ProjectionSecret)
	ac := rbacH.Checker()
	q := queue.New(cfg.RedisURL)
	if q != nil {
		defer q.Close()
	} else {
		slog.Warn("REDIS_URL unset; batch object ops and inventory enqueue return 503")
	}
	qc := quota.New(authStore, bucketStore, objectStore, uploadStore, settingsStore)
	bucketH := bucket.NewHandler(bucketStore, authH.RequireUser, s3c, cfg.CORSOrigins, ac, auditLog, settingsStore, s3cfg)
	objectH := objects.NewHandler(bucketStore, objectStore, s3c, authH.RequireUser, ac, auditLog, q, settingsStore, s3cfg)
	uploadH := uploads.NewHandler(s3c, bucketStore, objectStore, uploadStore, authH.RequireUser, ac, auditLog, qc, settingsStore, s3cfg)
	downloadH := downloads.NewHandler(s3c, bucketStore, authH.RequireUser, ac, auditLog, settingsStore, s3cfg)
	platformH := platform.NewHandler(settingsStore, authH.RequireUser, platform.Fallbacks{
		S3Endpoint:        cfg.S3Endpoint,
		DownloadCDNURL:    cfg.DownloadCDNURL,
		PreviewCDNURL:     cfg.PreviewCDNURL,
		ObjectHTTPDomain:  cfg.ObjectHTTPDomain,
		ObjectHTTPSDomain: cfg.ObjectHTTPSDomain,
	}, cfg.ProjectionSecret, q)
	projectH := project.NewHandler(cfg.ProjectionSecret, authStore, bucketStore, credsStore, q, objectStore)
	versionH := versions.NewHandler(versionStore, bucketStore, s3c, authH.RequireUser, ac, auditLog, settingsStore, s3cfg, func(ctx context.Context, bucket, key string) (bool, error) {
		if editStore == nil {
			return false, nil
		}
		lock, err := editStore.ActiveLock(ctx, bucket, key)
		return lock != nil, err
	})
	shareH := share.NewHandler(shareStore, bucketStore, s3c, authH.RequireUser, ac, auditLog, settingsStore, s3cfg)
	editH := edit.NewHandler(editStore, bucketStore, versionStore, settingsStore, s3c, authH.RequireUser, edit.OfficeEnv{
		URL: cfg.OfficeURL, JWTSecret: cfg.OfficeJWTSecret, PublicURL: cfg.PublicURL,
	}, ac, cfg.ProjectionSecret, s3cfg)
	credsH := creds.NewHandler(credsStore, authH.RequireUser, ac, cfg.S3Endpoint, cfg.S3Region, cfg.JWTSecret, bucketStore, objectStore, uploadStore, s3c, auditLog, qc, settingsStore, s3cfg)
	statsH := stats.NewHandler(statsStore, bucketStore, authStore, authH.RequireUser, ac)
	auditH := audit.NewHandler(auditStore, authH.RequireUser, ac, cfg.ProjectionSecret)
	var usageH *stats.InternalHandler
	if pool != nil {
		usageH = stats.NewInternalHandler(stats.NewUsageStore(pool), statsStore, cfg.ProjectionSecret)
	}
	previewH := preview.NewHandler(s3c, bucketStore, authH.RequireUser, ac, auditLog, settingsStore, s3cfg)
	if cfg.ProjectionSecret == "" {
		slog.Warn("PROJECTION_SECRET unset; internal projection routes return 503")
	}
	addr := cfg.HTTPAddr
	slog.Info("listen", "addr", addr)
	if err := http.ListenAndServe(addr, newMux(apiHandlers{
		auth: authH, bucket: bucketH, objects: objectH, uploads: uploadH, downloads: downloadH,
		platform: platformH, project: projectH, versions: versionH, share: shareH, edit: editH, rbac: rbacH, creds: credsH, stats: statsH, audit: auditH, preview: previewH, usage: usageH,
	})); err != nil {
		slog.Error("server", "err", err)
		os.Exit(1)
	}
}
