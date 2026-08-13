package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
	"github.com/cyxc1124/osspilot-tenant-api/internal/platform"
	"github.com/cyxc1124/osspilot-tenant-api/internal/queue"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

const (
	TaskInventory       = "objects:inventory"
	TaskInventoryBucket = queue.TaskInventoryBucket
	TaskTrash           = "objects:trash"
)

type Jobs struct {
	Buckets  *bucket.Store
	Objects  *objects.Store
	Settings *platform.Store
	S3       *storage.Client
}

func (j *Jobs) Inventory(ctx context.Context, _ *asynq.Task) error {
	items, err := j.Buckets.ListActive(ctx)
	if err != nil {
		return err
	}
	for _, b := range items {
		if err := j.scanBucket(ctx, b); err != nil {
			return fmt.Errorf("inventory %s: %w", b.BucketName, err)
		}
	}
	slog.Info("inventory done", "buckets", len(items))
	return nil
}

func (j *Jobs) InventoryBucket(ctx context.Context, t *asynq.Task) error {
	var req struct {
		BucketName string `json:"bucket_name"`
	}
	if err := json.Unmarshal(t.Payload(), &req); err != nil || strings.TrimSpace(req.BucketName) == "" {
		return fmt.Errorf("invalid inventory payload")
	}
	b, err := j.Buckets.GetByName(ctx, req.BucketName)
	if err != nil {
		return err
	}
	if b == nil || b.Status != "active" {
		return fmt.Errorf("bucket %s not found", req.BucketName)
	}
	if err := j.scanBucket(ctx, *b); err != nil {
		return fmt.Errorf("inventory %s: %w", req.BucketName, err)
	}
	slog.Info("inventory bucket done", "bucket", req.BucketName)
	return nil
}

func (j *Jobs) scanBucket(ctx context.Context, b bucket.Bucket) error {
	started := time.Now().UTC()
	token := ""
	for {
		page, err := j.S3.ListObjects(ctx, b.BucketName, token, 1000)
		if err != nil {
			return err
		}
		for _, obj := range page.Objects {
			if obj.Key == "" {
				continue
			}
			if err := j.Objects.UpsertSeen(ctx, b.ID, b.BucketName, obj.Key, obj.Size, obj.ETag, obj.StorageClass, started); err != nil {
				return err
			}
		}
		if !page.Truncated || page.Token == "" {
			break
		}
		token = page.Token
	}
	return j.Objects.PurgeUnseen(ctx, b.ID, started)
}

func (j *Jobs) Trash(ctx context.Context, _ *asynq.Task) error {
	if j.Settings == nil {
		return nil
	}
	days, enabled, err := j.Settings.TrashPolicy(ctx)
	if err != nil {
		return err
	}
	if !shouldCleanupTrash(enabled, days) {
		slog.Info("trash cleanup skipped", "enabled", enabled, "days", days)
		return nil
	}
	items, err := j.Objects.ExpiredTrash(ctx, days)
	if err != nil {
		return err
	}
	var deleted int
	for _, item := range items {
		if err := j.S3.DeleteObject(ctx, item.BucketName, item.Key); err != nil {
			slog.Warn("trash delete storage", "bucket", item.BucketName, "key", item.Key, "err", err)
			continue
		}
		if err := j.Objects.Delete(ctx, item.BucketID, item.Key); err != nil {
			return err
		}
		deleted++
	}
	slog.Info("trash cleanup done", "deleted", deleted)
	return nil
}

func shouldCleanupTrash(enabled bool, days int) bool {
	return enabled && days >= 1
}
