package preview

import (
	"path"
	"strings"
)

const maxTextBytes = 512 * 1024

var (
	imageExt = map[string]bool{"jpg": true, "jpeg": true, "png": true, "gif": true, "webp": true, "bmp": true, "svg": true}
	videoExt = map[string]bool{"mp4": true, "webm": true, "mov": true, "mkv": true}
	audioExt = map[string]bool{"mp3": true, "wav": true, "flac": true, "aac": true, "ogg": true}
	pdfExt   = map[string]bool{"pdf": true}
	textExt  = map[string]bool{
		"txt": true, "md": true, "json": true, "yaml": true, "yml": true, "xml": true,
		"csv": true, "log": true, "conf": true, "ini": true, "sh": true, "py": true,
		"js": true, "ts": true, "java": true, "go": true, "sql": true,
	}
	extLang = map[string]string{
		"py": "python", "js": "javascript", "ts": "typescript", "java": "java",
		"go": "go", "sql": "sql", "sh": "bash", "json": "json", "yaml": "yaml",
		"yml": "yaml", "xml": "xml", "md": "markdown", "csv": "csv", "ini": "ini",
		"conf": "ini", "log": "log", "txt": "text",
	}
)

func fileExt(key string) string {
	return strings.TrimPrefix(strings.ToLower(path.Ext(path.Base(key))), ".")
}

func filename(key string) string { return path.Base(key) }

func allowExt(key string, allowed map[string]bool) bool {
	return allowed[fileExt(key)]
}

func textOK(key string, contentType *string) bool {
	if textExt[fileExt(key)] {
		return true
	}
	if contentType == nil {
		return false
	}
	ct := strings.ToLower(*contentType)
	return strings.HasPrefix(ct, "text/") ||
		ct == "application/json" || ct == "application/xml" ||
		ct == "application/yaml" || ct == "application/x-yaml"
}

func guessLang(key string, contentType *string) string {
	if lang, ok := extLang[fileExt(key)]; ok {
		return lang
	}
	if contentType != nil {
		ct := strings.ToLower(*contentType)
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
	}
	return "text"
}
