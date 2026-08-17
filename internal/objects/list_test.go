package objects

import (
	"testing"

	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

func TestFoldList(t *testing.T) {
	recs := []record{
		{Key: "a.txt"},
		{Key: "docs/readme.md"},
		{Key: "docs/a/b.txt"},
		{Key: "z.txt"},
	}
	items, prefixes, token, truncated := foldList("", 10, recs, false)
	if truncated || token != nil {
		t.Fatalf("truncated=%v token=%v", truncated, token)
	}
	if len(items) != 2 || items[0].Key != "a.txt" || items[1].Key != "z.txt" {
		t.Fatalf("items %#v", items)
	}
	if len(prefixes) != 1 || prefixes[0] != "docs/" {
		t.Fatalf("prefixes %#v", prefixes)
	}

	items, prefixes, _, _ = foldList("docs/", 10, recs[1:3], false)
	if len(prefixes) != 1 || prefixes[0] != "docs/a/" {
		t.Fatalf("nested prefixes %#v", prefixes)
	}
	if len(items) != 1 || items[0].Key != "docs/readme.md" {
		t.Fatalf("nested items %#v", items)
	}

	_, _, token, truncated = foldList("", 1, recs, false)
	if !truncated || token == nil || *token != "a.txt" {
		t.Fatalf("page token=%v truncated=%v", token, truncated)
	}
}

func TestHiddenKey(t *testing.T) {
	if !hiddenKey(".trash/a") || !hiddenKey(".versions/a") {
		t.Fatal("hidden")
	}
	if hiddenKey("docs/a") {
		t.Fatal("visible")
	}
}

func TestDirectoryMarkerAndChildPrefix(t *testing.T) {
	if !isDirectoryMarker("docs/", 0) || isDirectoryMarker("docs/", 3) || isDirectoryMarker("docs", 0) {
		t.Fatal("marker")
	}
	if !isDirectChildPrefix("docs/", "") || !isDirectChildPrefix("docs/a/", "docs/") {
		t.Fatal("direct")
	}
	if isDirectChildPrefix("docs/a/b/", "docs/") || isDirectChildPrefix("docs/a.txt", "docs/") {
		t.Fatal("not direct")
	}
}

func TestCollectPrefixesAddsMarkers(t *testing.T) {
	page := storage.ListPage{
		Prefixes: []string{"docs/"},
		Objects: []storage.ListedObject{
			{Key: "keep/", Size: 0},
			{Key: ".trash/x/", Size: 0},
			{Key: "docs/nested/deep/", Size: 0},
		},
	}
	got := collectPrefixes(page, "")
	if len(got) != 2 || got[0] != "docs/" || got[1] != "keep/" {
		t.Fatalf("%#v", got)
	}
}

func TestLikePrefixEscapes(t *testing.T) {
	if got := likePrefix(`a%b_c\d`); got != `a\%b\_c\\d%` {
		t.Fatal(got)
	}
}
