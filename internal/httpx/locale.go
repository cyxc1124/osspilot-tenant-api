package httpx

import (
	"net/http"
	"strings"
)

const (
	localeHeader  = "X-App-Locale"
	defaultLocale = "zh-CN"
)

func ResolveLocale(r *http.Request) string {
	if r == nil {
		return defaultLocale
	}
	if v := strings.TrimSpace(r.Header.Get(localeHeader)); v != "" {
		return normalizeLocale(v)
	}
	if accept := r.Header.Get("Accept-Language"); accept != "" {
		first := strings.TrimSpace(strings.Split(strings.Split(accept, ",")[0], ";")[0])
		return normalizeLocale(first)
	}
	return defaultLocale
}

func normalizeLocale(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "zh-CN" || raw == "en-US" {
		return raw
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "zh") {
		return "zh-CN"
	}
	if strings.HasPrefix(lower, "en") {
		return "en-US"
	}
	return defaultLocale
}

type locWriter struct {
	http.ResponseWriter
	locale string
}

func (w locWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func localeOf(w http.ResponseWriter) string {
	for w != nil {
		switch t := w.(type) {
		case locWriter:
			return t.locale
		case *locWriter:
			return t.locale
		}
		u, ok := w.(interface{ Unwrap() http.ResponseWriter })
		if !ok {
			break
		}
		w = u.Unwrap()
	}
	return defaultLocale
}
