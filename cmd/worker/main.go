package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/config"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/platform"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
	"github.com/cyxc1124/osspilot-tenant-api/internal/uploads"
	"github.com/cyxc1124/osspilot-tenant-api/internal/versions"
	"github.com/cyxc1124/osspilot-tenant-api/internal/worker"
)

func main() {
	cfg := config.Load()
	if cfg.RedisURL == "" {
		slog.Error("REDIS_URL is required for the inventory worker")
		os.Exit(1)
	}
	if cfg.DatabaseURL == "" {
		slog.Error("DATABASE_URL is required for the inventory worker")
		os.Exit(1)
	}
	s3cfg := storage.Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.RGWAccessKey,
		SecretKey: cfg.RGWSecretKey,
		Region:    cfg.S3Region,
	}
	if !s3cfg.Ready() {
		slog.Error("S3_ENDPOINT/RGW_ACCESS_KEY/RGW_SECRET_KEY are required for the inventory worker")
		os.Exit(1)
	}

	redisOpt, err := asynq.ParseRedisURI(cfg.RedisURL)
	if err != nil {
		slog.Error("REDIS_URL", "err", err)
		os.Exit(1)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		slog.Error("db pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	jobs := &worker.Jobs{
		Buckets:  bucket.NewStore(pool),
		Objects:  objects.NewStore(pool),
		Versions: versions.NewStore(pool),
		Uploads:  uploads.NewStore(pool),
		Settings: platform.NewStore(pool),
		S3:       storage.New(s3cfg),
	}

	mux := asynq.NewServeMux()
	mux.HandleFunc(worker.TaskInventory, jobs.Inventory)
	mux.HandleFunc(worker.TaskInventoryBucket, jobs.InventoryBucket)
	mux.HandleFunc(worker.TaskTrash, jobs.Trash)
	mux.HandleFunc(worker.TaskVersions, jobs.CleanVersions)
	mux.HandleFunc(worker.TaskMultipart, jobs.CleanMultipart)

	srv := asynq.NewServer(redisOpt, asynq.Config{Concurrency: 2})
	scheduler := asynq.NewScheduler(redisOpt, nil)
	if _, err := scheduler.Register("@every 15m", asynq.NewTask(worker.TaskInventory, nil)); err != nil {
		slog.Error("schedule inventory", "err", err)
		os.Exit(1)
	}
	if _, err := scheduler.Register("@every 1h", asynq.NewTask(worker.TaskTrash, nil)); err != nil {
		slog.Error("schedule trash", "err", err)
		os.Exit(1)
	}
	if _, err := scheduler.Register("@every 6h", asynq.NewTask(worker.TaskVersions, nil)); err != nil {
		slog.Error("schedule versions", "err", err)
		os.Exit(1)
	}
	if _, err := scheduler.Register("@every 6h", asynq.NewTask(worker.TaskMultipart, nil)); err != nil {
		slog.Error("schedule multipart", "err", err)
		os.Exit(1)
	}

	go func() {
		if err := scheduler.Run(); err != nil {
			slog.Error("scheduler", "err", err)
			os.Exit(1)
		}
	}()

	client := asynq.NewClient(redisOpt)
	defer client.Close()
	if _, err := client.Enqueue(asynq.NewTask(worker.TaskInventory, nil), asynq.MaxRetry(3), asynq.Timeout(time.Hour)); err != nil {
		slog.Warn("enqueue startup inventory", "err", err)
	}

	slog.Info("worker listen")
	if err := srv.Run(mux); err != nil {
		slog.Error("worker", "err", err)
		os.Exit(1)
	}
}
