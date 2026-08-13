package versions

import (
	"context"
	"time"

	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

const SourceTextEdit = "text_edit"

func Archive(ctx context.Context, s3 *storage.Client, store *Store, userID int64, bucket, key, source string, remark *string) (int, error) {
	meta, err := s3.HeadObject(ctx, bucket, key)
	if err != nil {
		return 0, err
	}
	n, err := store.NextNo(ctx, bucket, key)
	if err != nil {
		return 0, err
	}
	sk := storageKey(bucket, key, n)
	if _, err := s3.CopyObject(ctx, bucket, sk, bucket, key); err != nil {
		return 0, err
	}
	rec := &Record{
		BucketName: bucket,
		ObjectKey:  key,
		VersionNo:  n,
		StorageKey: sk,
		Size:       meta.Size,
		ETag:       meta.ETag,
		CreatedBy:  userID,
		CreatedAt:  time.Now(),
		Source:     source,
		Remark:     remark,
	}
	if err := store.Insert(ctx, rec); err != nil {
		return 0, err
	}
	return n, nil
}
