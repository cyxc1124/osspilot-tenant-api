package creds

import "testing"

func TestParseAuthorization(t *testing.T) {
	a, err := parseAuthorization("OSSAccessKey OSSABC:secretvalue")
	if err != nil || a.Type != "access_key" || a.KeyID != "OSSABC" || a.Secret != "secretvalue" {
		t.Fatalf("access key %#v %v", a, err)
	}
	if _, err := parseAuthorization("OSSAccessKey OSSABC:secret:extra"); err == nil {
		t.Fatal("extra token should fail")
	}
	s, err := parseAuthorization("OSSSession kid:sec:tok")
	if err != nil || s.Type != "sts_session" || s.Session != "tok" {
		t.Fatalf("session %#v %v", s, err)
	}
	if _, err := parseAuthorization("Bearer abc"); err == nil {
		t.Fatal("bearer should fail")
	}
	if _, err := parseAuthorization(""); err == nil {
		t.Fatal("empty should fail")
	}
}
