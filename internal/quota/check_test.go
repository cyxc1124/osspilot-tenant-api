package quota

import "testing"

func TestCheckTenantQuota(t *testing.T) {
	q := int64(1000)
	err := Check(Input{Size: 100, Account: Limits{QuotaBytes: &q}, AccountUsed: 950})
	if err == nil || err.Error() != "Tenant quota exceeded" {
		t.Fatalf("got %v", err)
	}
	if err := Check(Input{Size: 50, Account: Limits{QuotaBytes: &q}, AccountUsed: 950}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckTenantObjectLimitIgnoresOverwrite(t *testing.T) {
	n := int64(1)
	in := Input{Size: 10, Account: Limits{ObjectLimit: &n}, AccountCount: 1, NewObject: true}
	if err := Check(in); err == nil || err.Error() != "Tenant object limit exceeded" {
		t.Fatalf("new %v", err)
	}
	in.NewObject = false
	if err := Check(in); err != nil {
		t.Fatal(err)
	}
}

func TestCheckDailyAndBucket(t *testing.T) {
	d := int64(1000)
	if err := Check(Input{Size: 300, Account: Limits{DailyUploadBytes: &d}, AccountDaily: 800}); err == nil || err.Error() != "Tenant daily upload quota exceeded" {
		t.Fatalf("daily %v", err)
	}
	b := int64(1000)
	if err := Check(Input{Size: 200, Bucket: Limits{QuotaBytes: &b}, BucketUsed: 900}); err == nil || err.Error() != "Bucket quota exceeded" {
		t.Fatalf("bucket %v", err)
	}
	n := int64(1)
	if err := Check(Input{Size: 1, NewObject: true, Bucket: Limits{ObjectLimit: &n}, BucketCount: 1}); err == nil || err.Error() != "Bucket object limit exceeded" {
		t.Fatalf("bucket count %v", err)
	}
}

func TestCheckMaxUploadBytes(t *testing.T) {
	n := int64(100)
	if err := Check(Input{Size: 101, MaxUploadBytes: &n}); err == nil || err.Error() != "File size exceeds maximum allowed (100 bytes)" {
		t.Fatalf("got %v", err)
	}
	if err := Check(Input{Size: 100, MaxUploadBytes: &n}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckNilLimitsPass(t *testing.T) {
	if err := Check(Input{Size: 1 << 40, NewObject: true, AccountUsed: 1 << 40, BucketUsed: 1 << 40}); err != nil {
		t.Fatal(err)
	}
}
