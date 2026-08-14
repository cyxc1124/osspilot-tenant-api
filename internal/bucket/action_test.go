package bucket

import "testing"

func TestUpdateAction(t *testing.T) {
	q := int64(100)
	before := Bucket{QuotaBytes: &q, AccessLoggingEnabled: false}
	on := true
	if got := updateAction(updateRequest{AccessLoggingEnabled: &on}, before); got != "modify_access_logging" {
		t.Fatalf("logging: %s", got)
	}
	nq := int64(200)
	if got := updateAction(updateRequest{QuotaBytes: &nq}, before); got != "modify_bucket_quota" {
		t.Fatalf("quota: %s", got)
	}
	alias := true
	if got := updateAction(updateRequest{DisplayAliasOnly: &alias}, before); got != "modify_bucket" {
		t.Fatalf("other: %s", got)
	}
}
