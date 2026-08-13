package rbac

import "testing"

func TestProjectionBearerOK(t *testing.T) {
	if !projectionBearerOK("Bearer secret", "secret") {
		t.Fatal("expected match")
	}
	if projectionBearerOK("Bearer nope", "secret") || projectionBearerOK("secret", "secret") || projectionBearerOK("Bearer secret", "") {
		t.Fatal("expected reject")
	}
}
