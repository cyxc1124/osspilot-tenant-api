package objects

import "testing"

func TestValidUserKey(t *testing.T) {
	if !ValidUserKey("docs/a.txt") {
		t.Fatal("docs/a.txt")
	}
	for _, k := range []string{"", ".trash", ".versions", ".trash/a", ".versions/x"} {
		if ValidUserKey(k) {
			t.Fatalf("accepted %q", k)
		}
	}
}
