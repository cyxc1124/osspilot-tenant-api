package edit

import "testing"

func TestInternalBearerOK(t *testing.T) {
	if !internalBearerOK("Bearer secret", "secret") {
		t.Fatal("ok")
	}
	if internalBearerOK("Bearer nope", "secret") || internalBearerOK("secret", "secret") {
		t.Fatal("reject")
	}
}
