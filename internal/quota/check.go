package quota

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
)

type Exceeded struct {
	Detail string
}

func (e *Exceeded) Error() string { return e.Detail }

type Limits struct {
	QuotaBytes       *int64
	ObjectLimit      *int64
	DailyUploadBytes *int64
}

type Input struct {
	Size           int64
	NewObject      bool
	MaxUploadBytes *int64
	Account        Limits
	Bucket         Limits
	AccountUsed    int64
	AccountCount   int64
	AccountDaily   int64
	BucketUsed     int64
	BucketCount    int64
}

func Check(in Input) error {
	if in.MaxUploadBytes != nil && in.Size > *in.MaxUploadBytes {
		return &Exceeded{Detail: "File size exceeds maximum allowed (" + strconv.FormatInt(*in.MaxUploadBytes, 10) + " bytes)"}
	}
	if overBytes(in.Account.QuotaBytes, in.AccountUsed, in.Size) {
		return &Exceeded{Detail: "Tenant quota exceeded"}
	}
	if in.NewObject && overCount(in.Account.ObjectLimit, in.AccountCount) {
		return &Exceeded{Detail: "Tenant object limit exceeded"}
	}
	if overBytes(in.Account.DailyUploadBytes, in.AccountDaily, in.Size) {
		return &Exceeded{Detail: "Tenant daily upload quota exceeded"}
	}
	if overBytes(in.Bucket.QuotaBytes, in.BucketUsed, in.Size) {
		return &Exceeded{Detail: "Bucket quota exceeded"}
	}
	if in.NewObject && overCount(in.Bucket.ObjectLimit, in.BucketCount) {
		return &Exceeded{Detail: "Bucket object limit exceeded"}
	}
	return nil
}

func overBytes(limit *int64, used, add int64) bool {
	return limit != nil && used+add > *limit
}

func overCount(limit *int64, used int64) bool {
	return limit != nil && used+1 > *limit
}

func Reject(w http.ResponseWriter, err error) {
	var e *Exceeded
	if errors.As(err, &e) {
		httpx.Error(w, http.StatusRequestEntityTooLarge, e.Detail)
		return
	}
	httpx.Error(w, http.StatusInternalServerError, "database error")
}
