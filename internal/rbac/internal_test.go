package rbac

import "testing"

func TestProjectionOK(t *testing.T) {
	if !projectionOK("Bearer secret", "secret") {
		t.Fatal("want match")
	}
	if projectionOK("Bearer nope", "secret") || projectionOK("secret", "secret") {
		t.Fatal("want reject")
	}
}
