package share

import (
	"crypto/rand"
	"encoding/base64"
	"time"
)

const (
	tokenBytes        = 32
	defaultShareTTL   = 7 * 24 * time.Hour
	defaultPresignCap = 10 * time.Minute
)

func newToken() (string, error) {
	b := make([]byte, tokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func sharePath(token string) string {
	return "/s/" + token
}

func presignTTL(expiresAt *time.Time, now time.Time, cap time.Duration) time.Duration {
	if cap <= 0 {
		cap = defaultPresignCap
	}
	ttl := cap
	if expiresAt != nil {
		rem := expiresAt.Sub(now)
		if rem <= 0 {
			return 0
		}
		if rem < ttl {
			ttl = rem
		}
	}
	if ttl < time.Second {
		return 0
	}
	return ttl
}
