package objects

import "testing"

func TestRenameDest(t *testing.T) {
	got, err := renameDest("docs/a.txt", "b.txt")
	if err != nil || got != "docs/b.txt" {
		t.Fatalf("%q %v", got, err)
	}
	got, err = renameDest("a.txt", "b.txt")
	if err != nil || got != "b.txt" {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := renameDest("a.txt", "x/y"); err == nil {
		t.Fatal("expected error")
	}
}

func TestCopyErrDestExists(t *testing.T) {
	if CopyErr(ErrDestExists) != "Destination already exists" {
		t.Fatal(CopyErr(ErrDestExists))
	}
}
