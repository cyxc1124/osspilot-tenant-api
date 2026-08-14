package quota

import (
	"context"
	"fmt"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/bucket"
	"github.com/cyxc1124/osspilot-tenant-api/internal/objects"
)

type dailyStore interface {
	CompletedBytesSince(ctx context.Context, accountID int64, since time.Time) (int64, error)
}

type Checker struct {
	users   *auth.Store
	buckets *bucket.Store
	objects *objects.Store
	daily   dailyStore
}

func New(users *auth.Store, buckets *bucket.Store, objects *objects.Store, daily dailyStore) *Checker {
	if users == nil || buckets == nil || objects == nil || daily == nil {
		return nil
	}
	return &Checker{users: users, buckets: buckets, objects: objects, daily: daily}
}

func (c *Checker) Upload(ctx context.Context, accountID int64, b *bucket.Bucket, key string, size int64) error {
	if c == nil || b == nil {
		return nil
	}
	acc, err := c.users.GetByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("account: %w", err)
	}
	exists, err := c.objects.Exists(ctx, b.ID, key)
	if err != nil {
		return err
	}
	items, err := c.buckets.List(ctx, accountID)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(items)+1)
	seen := map[int64]struct{}{}
	for _, item := range items {
		ids = append(ids, item.ID)
		seen[item.ID] = struct{}{}
	}
	if _, ok := seen[b.ID]; !ok {
		ids = append(ids, b.ID)
	}
	usage, err := c.buckets.UsageByID(ctx, ids)
	if err != nil {
		return err
	}
	var accountUsed, accountCount int64
	for _, u := range usage {
		accountUsed += u.UsedBytes
		accountCount += u.ObjectCount
	}
	bu := usage[b.ID]
	now := time.Now().UTC()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	daily, err := c.daily.CompletedBytesSince(ctx, accountID, dayStart)
	if err != nil {
		return err
	}
	in := Input{
		Size: size, NewObject: !exists,
		AccountUsed: accountUsed, AccountCount: accountCount, AccountDaily: daily,
		BucketUsed: bu.UsedBytes, BucketCount: bu.ObjectCount,
		Bucket: Limits{QuotaBytes: b.QuotaBytes, ObjectLimit: b.ObjectLimit},
	}
	if acc != nil {
		in.Account = Limits{QuotaBytes: acc.QuotaBytes, ObjectLimit: acc.ObjectLimit, DailyUploadBytes: acc.DailyUploadBytes}
	}
	return Check(in)
}
