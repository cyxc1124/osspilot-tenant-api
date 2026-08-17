package stats

import (
	"math"
	"strings"
)

func usagePercent(used int64, quota *int64) *float64 {
	if quota == nil || *quota <= 0 {
		return nil
	}
	v := math.Round(float64(used)/float64(*quota)*10000) / 10000
	return &v
}

func remainingBytes(quota *int64, used int64) *int64 {
	if quota == nil {
		return nil
	}
	n := *quota - used
	if n < 0 {
		n = 0
	}
	return &n
}

func displayPrefix(prefix string) string {
	if prefix == "" {
		return "(根目录)"
	}
	return prefix
}

func parsePeriod(raw string) (string, bool) {
	if raw == "" {
		return "24h", true
	}
	switch strings.ToLower(raw) {
	case "24h", "7d", "30d":
		return strings.ToLower(raw), true
	}
	return "", false
}
