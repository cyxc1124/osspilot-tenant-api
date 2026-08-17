package project

import "testing"

func TestBearerOK(t *testing.T) {
	if !bearerOK("Bearer secret", "secret") {
		t.Fatal("want match")
	}
	if bearerOK("Bearer nope", "secret") || bearerOK("secret", "secret") || bearerOK("Bearer secret", "") {
		t.Fatal("want reject")
	}
}
