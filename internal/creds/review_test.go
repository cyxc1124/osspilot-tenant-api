package creds

import "testing"

func TestReviewTarget(t *testing.T) {
	got, err := reviewTarget("approve", "pending")
	if err != nil || got != "approved" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := reviewTarget("approve", "approved"); err == nil {
		t.Fatal("expected error")
	}
	got, err = reviewTarget("disable", "approved")
	if err != nil || got != "disabled" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := reviewTarget("reject", "disabled"); err == nil {
		t.Fatal("expected error")
	}
}
