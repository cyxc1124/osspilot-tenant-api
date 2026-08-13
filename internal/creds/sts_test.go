package creds

import (
	"testing"
	"time"
)

func TestClampSTS(t *testing.T) {
	if got := clampSTS(100); got != minSTS {
		t.Fatalf("min got %d", got)
	}
	if got := clampSTS(3600); got != 3600 {
		t.Fatalf("default got %d", got)
	}
	if got := clampSTS(99_999); got != maxSTS {
		t.Fatalf("max got %d", got)
	}
}

func TestSTSRoundTrip(t *testing.T) {
	secret := "test-secret"
	key := &Key{ID: 7, AccessKeyID: "OSSABC", AccountID: 3, ApplicationID: 9}
	exp := time.Now().Add(time.Hour)
	tok, err := signSTS(secret, key, exp)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := parseSTS(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Kind != stsKind || claims.AccessKeyID != key.AccessKeyID || claims.AccountID != key.AccountID || claims.ApplicationID != key.ApplicationID {
		t.Fatalf("claims %#v", claims)
	}
	if _, err := parseSTS("other", tok); err == nil {
		t.Fatal("wrong secret should fail")
	}
}
