package creds

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	schemeAccessKey = "OSSAccessKey"
	schemeSession   = "OSSSession"
)

type parsedAuth struct {
	Type    string
	KeyID   string
	Secret  string
	Session string
}

func parseAuthorization(header string) (parsedAuth, error) {
	header = strings.TrimSpace(header)
	if header == "" {
		return parsedAuth{}, fmt.Errorf("Missing Authorization header")
	}
	space := strings.IndexByte(header, ' ')
	if space < 0 {
		return parsedAuth{}, fmt.Errorf("Unsupported Authorization scheme; use OSSAccessKey or OSSSession")
	}
	scheme, rest := header[:space], strings.TrimSpace(header[space+1:])
	parts := strings.Split(rest, ":")
	switch {
	case strings.EqualFold(scheme, schemeAccessKey):
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return parsedAuth{}, fmt.Errorf("OSSAccessKey must not include a session token")
		}
		return parsedAuth{Type: "access_key", KeyID: parts[0], Secret: parts[1]}, nil
	case strings.EqualFold(scheme, schemeSession):
		if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return parsedAuth{}, fmt.Errorf("OSSSession requires access_key_id:secret_access_key:session_token")
		}
		return parsedAuth{Type: "sts_session", KeyID: parts[0], Secret: parts[1], Session: parts[2]}, nil
	default:
		return parsedAuth{}, fmt.Errorf("Unsupported Authorization scheme; use OSSAccessKey or OSSSession")
	}
}

func newAccessKeyID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "OSS" + strings.ToUpper(hex.EncodeToString(b)), nil
}

func newSecretKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
