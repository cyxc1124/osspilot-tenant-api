package objects

import (
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/queue"
)

// ponytail: same threshold as legacy BATCH_ASYNC_MIN_KEYS; a single fat directory also queues.
const batchAsyncMinKeys = 2

func shouldQueue(n int) bool { return n >= batchAsyncMinKeys }

func (h *Handler) queueDelete(w http.ResponseWriter, r *http.Request, user *auth.User, bucketName string, keys []string, permanent bool, n int) bool {
	ip, ua := requestMeta(r)
	id, err := h.q.EnqueueBatchDelete(r.Context(), queue.BatchDelete{
		AccountID: auth.AccountID(user), UserID: user.ID, BucketName: bucketName,
		Keys: keys, Permanent: permanent, SourceIP: ip, UserAgent: ua,
	})
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, queueDetail(err))
		return true
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"deleted": []string{}, "failed": []opFailure{}, "status": "queued", "job_id": id, "queued_count": n,
	})
	return true
}

func (h *Handler) queueCopy(w http.ResponseWriter, r *http.Request, user *auth.User, bucketName string, items []keyPair) bool {
	ip, ua := requestMeta(r)
	id, err := h.q.EnqueueBatchCopy(r.Context(), queue.BatchCopy{
		AccountID: auth.AccountID(user), UserID: user.ID, BucketName: bucketName,
		Items: toQueueItems(items), SourceIP: ip, UserAgent: ua,
	})
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, queueDetail(err))
		return true
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"copied": []any{}, "failed": []any{}, "status": "queued", "job_id": id, "queued_count": len(items),
	})
	return true
}

func (h *Handler) queueMove(w http.ResponseWriter, r *http.Request, user *auth.User, bucketName string, items []keyPair) bool {
	ip, ua := requestMeta(r)
	id, err := h.q.EnqueueBatchMove(r.Context(), queue.BatchMove{
		AccountID: auth.AccountID(user), UserID: user.ID, BucketName: bucketName,
		Items: toQueueItems(items), SourceIP: ip, UserAgent: ua,
	})
	if err != nil {
		httpx.Error(w, http.StatusServiceUnavailable, queueDetail(err))
		return true
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"moved": []any{}, "failed": []any{}, "status": "queued", "job_id": id, "queued_count": len(items),
	})
	return true
}

func (h *Handler) missingQueue(w http.ResponseWriter, n int) bool {
	if !shouldQueue(n) || h.q != nil {
		return false
	}
	httpx.Error(w, http.StatusServiceUnavailable, "REDIS_URL is not configured")
	return true
}

func toQueueItems(items []keyPair) []queue.CopyItem {
	out := make([]queue.CopyItem, len(items))
	for i, it := range items {
		out[i] = queue.CopyItem{SourceKey: it.SourceKey, DestKey: it.DestKey, DestBucketName: it.DestBucketName}
	}
	return out
}

func queueDetail(err error) string {
	if errors.Is(err, queue.ErrUnavailable) {
		return "REDIS_URL is not configured"
	}
	return "Failed to enqueue batch task"
}

func requestMeta(r *http.Request) (string, string) {
	ip := ""
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip = strings.TrimSpace(strings.Split(xff, ",")[0])
	} else if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		ip = host
	} else {
		ip = r.RemoteAddr
	}
	return ip, r.Header.Get("User-Agent")
}
