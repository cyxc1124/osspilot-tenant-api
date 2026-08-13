package bucket

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

var allowedCORSMethods = map[string]bool{
	"GET": true, "PUT": true, "POST": true, "DELETE": true, "HEAD": true,
}

type corsRule struct {
	AllowedOrigins []string `json:"allowed_origins"`
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`
	ExposeHeaders  []string `json:"expose_headers"`
	MaxAgeSeconds  *int     `json:"max_age_seconds"`
}

func validateCorsRules(rules []corsRule) ([]corsRule, error) {
	if len(rules) == 0 {
		return nil, fmt.Errorf("cors_rules must contain at least one rule")
	}
	out := make([]corsRule, 0, len(rules))
	for _, rule := range rules {
		n, err := validateCorsRule(rule)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, nil
}

func validateCorsRule(rule corsRule) (corsRule, error) {
	if len(rule.AllowedOrigins) == 0 {
		return corsRule{}, fmt.Errorf("allowed_origins must be a non-empty list")
	}
	origins := make([]string, 0, len(rule.AllowedOrigins))
	for _, raw := range rule.AllowedOrigins {
		o, err := validateOrigin(raw)
		if err != nil {
			return corsRule{}, err
		}
		origins = append(origins, o)
	}
	methods, err := validateMethods(rule.AllowedMethods)
	if err != nil {
		return corsRule{}, err
	}
	headers := rule.AllowedHeaders
	if headers == nil {
		headers = []string{"*"}
	}
	cleaned, err := cleanStrings(headers, "allowed_headers", false)
	if err != nil {
		return corsRule{}, err
	}
	expose, err := cleanStrings(rule.ExposeHeaders, "expose_headers", true)
	if err != nil {
		return corsRule{}, err
	}
	if rule.MaxAgeSeconds != nil && *rule.MaxAgeSeconds < 0 {
		return corsRule{}, fmt.Errorf("max_age_seconds must be a non-negative integer")
	}
	out := corsRule{AllowedOrigins: origins, AllowedMethods: methods}
	out.AllowedHeaders = cleaned
	if len(expose) > 0 {
		out.ExposeHeaders = expose
	}
	out.MaxAgeSeconds = rule.MaxAgeSeconds
	return out, nil
}

func validateOrigin(origin string) (string, error) {
	value := strings.TrimSpace(origin)
	if value == "" {
		return "", fmt.Errorf("allowed_origins must not contain empty values")
	}
	if value == "*" {
		return value, nil
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("Invalid CORS origin %q; use * or a full http(s) URL", origin)
	}
	return value, nil
}

func validateMethods(methods []string) ([]string, error) {
	if len(methods) == 0 {
		return nil, fmt.Errorf("allowed_methods must contain at least one method")
	}
	var out []string
	seen := map[string]bool{}
	for _, m := range methods {
		upper := strings.ToUpper(strings.TrimSpace(m))
		if !allowedCORSMethods[upper] {
			return nil, fmt.Errorf("Unsupported CORS method %q; allowed: DELETE, GET, HEAD, POST, PUT", m)
		}
		if seen[upper] {
			continue
		}
		seen[upper] = true
		out = append(out, upper)
	}
	return out, nil
}

func cleanStrings(values []string, field string, allowEmpty bool) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	var out []string
	for _, v := range values {
		s := strings.TrimSpace(v)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	if !allowEmpty && len(out) == 0 {
		return nil, fmt.Errorf("%s must not be empty when provided", field)
	}
	return out, nil
}

func defaultCorsRules(origins []string) []corsRule {
	if len(origins) == 0 {
		return nil
	}
	age := 3600
	return []corsRule{{
		AllowedOrigins: append([]string(nil), origins...),
		AllowedMethods: []string{"GET", "PUT", "POST", "HEAD"},
		AllowedHeaders: []string{"*"},
		ExposeHeaders:  []string{"ETag"},
		MaxAgeSeconds:  &age,
	}}
}

func toStorageCORS(rules []corsRule) []storage.CORSRule {
	out := make([]storage.CORSRule, 0, len(rules))
	for _, r := range rules {
		item := storage.CORSRule{
			AllowedOrigins: r.AllowedOrigins,
			AllowedMethods: r.AllowedMethods,
			AllowedHeaders: r.AllowedHeaders,
			ExposeHeaders:  r.ExposeHeaders,
		}
		if r.MaxAgeSeconds != nil {
			n := int32(*r.MaxAgeSeconds)
			item.MaxAgeSeconds = &n
		}
		out = append(out, item)
	}
	return out
}

func fromStorageCORS(rules []storage.CORSRule) []corsRule {
	out := make([]corsRule, 0, len(rules))
	for _, r := range rules {
		item := corsRule{
			AllowedOrigins: r.AllowedOrigins,
			AllowedMethods: r.AllowedMethods,
			AllowedHeaders: r.AllowedHeaders,
			ExposeHeaders:  r.ExposeHeaders,
		}
		if len(item.AllowedHeaders) == 0 {
			item.AllowedHeaders = []string{"*"}
		}
		if item.ExposeHeaders == nil {
			item.ExposeHeaders = []string{}
		}
		if r.MaxAgeSeconds != nil {
			n := int(*r.MaxAgeSeconds)
			item.MaxAgeSeconds = &n
		}
		out = append(out, item)
	}
	return out
}
