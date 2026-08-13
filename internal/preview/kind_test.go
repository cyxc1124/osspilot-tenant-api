package preview

import "testing"

func TestAllowExtAndTextOK(t *testing.T) {
	if !allowExt("a/b.PNG", imageExt) {
		t.Fatal("png")
	}
	if allowExt("a/b.txt", imageExt) {
		t.Fatal("txt not image")
	}
	ct := "application/json"
	if !textOK("data.bin", &ct) {
		t.Fatal("json content-type")
	}
	if guessLang("x.go", nil) != "go" {
		t.Fatal("go lang")
	}
}
