package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"
)

const TaskInventoryBucket = "objects:inventory_bucket"

var ErrUnavailable = errors.New("task queue unavailable")

type Client struct {
	asynq *asynq.Client
}

func New(redisURL string) *Client {
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		return nil
	}
	opt, err := asynq.ParseRedisURI(redisURL)
	if err != nil {
		return nil
	}
	return &Client{asynq: asynq.NewClient(opt)}
}

func (c *Client) Close() error {
	if c == nil || c.asynq == nil {
		return nil
	}
	return c.asynq.Close()
}

func (c *Client) EnqueueInventory(ctx context.Context, bucketName string) (string, error) {
	if c == nil || c.asynq == nil {
		return "", ErrUnavailable
	}
	bucketName = strings.TrimSpace(bucketName)
	if bucketName == "" {
		return "", fmt.Errorf("bucket_name is required")
	}
	payload, err := json.Marshal(map[string]string{"bucket_name": bucketName})
	if err != nil {
		return "", err
	}
	info, err := c.asynq.EnqueueContext(ctx, asynq.NewTask(TaskInventoryBucket, payload),
		asynq.MaxRetry(3), asynq.Timeout(time.Hour), asynq.Queue("default"))
	if err != nil {
		return "", err
	}
	return info.ID, nil
}
