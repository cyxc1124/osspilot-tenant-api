package versions

import "testing"

func TestStorageKey(t *testing.T) {
	got := storageKey("demo-app-dev", "docs/config.py", 1)
	want := ".versions/demo-app-dev/docs%2Fconfig.py/v1"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if downloadName("docs/config.py", 3) != "config.py.v3" {
		t.Fatal(downloadName("docs/config.py", 3))
	}
}
