package share

import (
	"testing"
	"time"
)

func TestSharePath(t *testing.T) {
	if got := sharePath("abc"); got != "/s/abc" {
		t.Fatal(got)
	}
}

func TestNewToken(t *testing.T) {
	a, err := newToken()
	if err != nil || a == "" {
		t.Fatalf("a=%q err=%v", a, err)
	}
	b, err := newToken()
	if err != nil || a == b {
		t.Fatalf("collision a=%q b=%q err=%v", a, b, err)
	}
}

func TestPresignTTL(t *testing.T) {
	now := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	cap := 10 * time.Minute
	if got := presignTTL(nil, now, cap); got != cap {
		t.Fatalf("nil expires: %v", got)
	}
	past := now.Add(-time.Minute)
	if got := presignTTL(&past, now, cap); got != 0 {
		t.Fatalf("past: %v", got)
	}
	soon := now.Add(30 * time.Second)
	if got := presignTTL(&soon, now, cap); got != 30*time.Second {
		t.Fatalf("soon: %v", got)
	}
	later := now.Add(time.Hour)
	if got := presignTTL(&later, now, cap); got != cap {
		t.Fatalf("later: %v", got)
	}
}
