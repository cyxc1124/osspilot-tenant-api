package edit

import "testing"

func TestFileExt(t *testing.T) {
	if fileExt("docs/a.PY") != "py" || fileExt("README") != "" || fileExt("a.") != "" {
		t.Fatal("ext")
	}
}

func TestGuessLanguage(t *testing.T) {
	if guessLanguage("app.py", "") != "python" {
		t.Fatal("py")
	}
	if guessLanguage("x.bin", "application/json") != "json" {
		t.Fatal("json ct")
	}
	if guessLanguage("x.bin", "text/plain") != "text" {
		t.Fatal("text ct")
	}
}

func TestTextEditable(t *testing.T) {
	if !textEditable("a.md", "") || !textEditable("x", "text/plain") || textEditable("a.docx", "") {
		t.Fatal("editable")
	}
}

func TestOfficeExt(t *testing.T) {
	if _, ok := officeExts["docx"]; !ok {
		t.Fatal("docx")
	}
	if _, ok := officeExts["txt"]; ok {
		t.Fatal("txt")
	}
}
