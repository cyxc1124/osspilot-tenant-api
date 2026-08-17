package edit

import (
	"path"
	"strings"
)

var textExts = map[string]string{
	"txt":  "text",
	"md":   "markdown",
	"json": "json",
	"yaml": "yaml",
	"yml":  "yaml",
	"xml":  "xml",
	"conf": "ini",
	"ini":  "ini",
	"sh":   "bash",
	"py":   "python",
	"js":   "javascript",
	"ts":   "typescript",
	"go":   "go",
	"java": "java",
	"sql":  "sql",
}

func fileExt(key string) string {
	base := path.Base(key)
	i := strings.LastIndex(base, ".")
	if i < 0 || i == len(base)-1 {
		return ""
	}
	return strings.ToLower(base[i+1:])
}

func guessLanguage(key, contentType string) string {
	if lang, ok := textExts[fileExt(key)]; ok {
		return lang
	}
	ct := strings.ToLower(contentType)
	switch {
	case strings.Contains(ct, "json"):
		return "json"
	case strings.Contains(ct, "xml"):
		return "xml"
	case strings.Contains(ct, "yaml"):
		return "yaml"
	case strings.HasPrefix(ct, "text/"):
		return "text"
	}
	return "text"
}

func textEditable(key, contentType string) bool {
	if _, ok := textExts[fileExt(key)]; ok {
		return true
	}
	ct := strings.ToLower(contentType)
	return strings.HasPrefix(ct, "text/") || ct == "application/json" || ct == "application/xml" || ct == "application/yaml" || ct == "application/x-yaml"
}

type officeKind struct {
	DocType     string
	FileType    string
	ContentType string
}

var officeExts = map[string]officeKind{
	"docx": {"word", "docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	"xlsx": {"cell", "xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	"pptx": {"slide", "pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
}
