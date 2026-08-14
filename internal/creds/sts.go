package creds

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/cyxc1124/osspilot-tenant-api/internal/auth"
	"github.com/cyxc1124/osspilot-tenant-api/internal/httpx"
	"github.com/cyxc1124/osspilot-tenant-api/internal/storage"
)

const (
	defaultSTS = 3600
	minSTS     = 900
	maxSTS     = 12 * 3600
	stsKind    = "sts"
)

type stsClaims struct {
	Kind          string `json:"kind"`
	AccessKeyID   string `json:"access_key_id"`
	AccountID     int64  `json:"account_id"`
	ApplicationID int64  `json:"application_id"`
	jwt.RegisteredClaims
}

type stsReq struct {
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	DurationSeconds int    `json:"duration_seconds"`
	TenantID        *int64 `json:"tenant_id"`
}

func (h *Handler) authSTS(r *http.Request, parsed parsedAuth) (Principal, error) {
	if claims, err := parseSTS(h.secret, parsed.Session); err == nil {
		key, err := h.store.GetKeyByAccessID(r.Context(), claims.AccessKeyID)
		if err != nil {
			return Principal{}, statusError{http.StatusInternalServerError, "database error"}
		}
		if key == nil || key.AccountID != claims.AccountID {
			return Principal{}, statusError{http.StatusUnauthorized, "Invalid STS session credentials"}
		}
		return h.principalFromKey(r, key, "sts_session")
	}
	if h.s3End == "" || parsed.Session == "" {
		return Principal{}, statusError{http.StatusUnauthorized, "Invalid STS session credentials"}
	}
	uid, err := storage.CallerAccount(r.Context(), h.s3End, h.s3Reg, parsed.KeyID, parsed.Secret, parsed.Session)
	if err != nil {
		return Principal{}, statusError{http.StatusUnauthorized, "Invalid STS session credentials"}
	}
	access, err := h.store.GetAccessByRGWUID(r.Context(), uid)
	if err != nil {
		return Principal{}, statusError{http.StatusInternalServerError, "database error"}
	}
	if access == nil || access.Status != "approved" {
		return Principal{}, statusError{http.StatusForbidden, "STS session tenant is not approved for API access"}
	}
	apps, err := h.store.ListApps(r.Context(), access.AccountID)
	if err != nil {
		return Principal{}, statusError{http.StatusInternalServerError, "database error"}
	}
	var app *App
	for i := range apps {
		if apps[i].Status == "active" {
			app = &apps[i]
			break
		}
	}
	if app == nil {
		return Principal{}, statusError{http.StatusForbidden, "STS session cannot be attributed to a tenant application"}
	}
	return Principal{
		AccountID: access.AccountID, ApplicationID: app.ID, ActingUserID: app.CreatedBy,
		AccessKeyID: parsed.KeyID, AuthType: "sts_session",
	}, nil
}

func (h *Handler) principalFromKey(r *http.Request, key *Key, authType string) (Principal, error) {
	if key.Status != "active" {
		return Principal{}, statusError{http.StatusForbidden, "Access key is disabled"}
	}
	app, err := h.store.GetApp(r.Context(), key.AccountID, key.ApplicationID)
	if err != nil || app == nil {
		return Principal{}, statusError{http.StatusInternalServerError, "database error"}
	}
	if app.Status != "active" {
		return Principal{}, statusError{http.StatusForbidden, "Application is disabled"}
	}
	access, err := h.store.GetAccess(r.Context(), key.AccountID)
	if err != nil {
		return Principal{}, statusError{http.StatusInternalServerError, "database error"}
	}
	if access == nil || access.Status != "approved" {
		return Principal{}, statusError{http.StatusForbidden, "Tenant API access is not approved"}
	}
	_ = h.store.TouchKey(r.Context(), key.ID)
	return Principal{
		AccountID: key.AccountID, ApplicationID: key.ApplicationID, ActingUserID: app.CreatedBy,
		AccessKeyID: key.AccessKeyID, AuthType: authType,
	}, nil
}

func clampSTS(n int) int {
	if n < minSTS {
		return minSTS
	}
	if n > maxSTS {
		return maxSTS
	}
	return n
}

func (h *Handler) issueSTS(w http.ResponseWriter, r *http.Request, user *auth.User) {
	accountID, ok := h.gate(w, r, user)
	if !ok || !h.requireApproved(w, r, accountID) {
		return
	}
	var req stsReq
	if err := httpx.DecodeJSON(r, &req); err != nil {
		httpx.Error(w, http.StatusBadRequest, "invalid json")
		return
	}
	if req.TenantID != nil && *req.TenantID != accountID {
		httpx.Error(w, http.StatusForbidden, "Tenant access denied")
		return
	}
	if len(req.AccessKeyID) < 8 || req.SecretAccessKey == "" {
		httpx.Error(w, http.StatusBadRequest, "access_key_id and secret_access_key are required")
		return
	}
	dur := req.DurationSeconds
	if dur == 0 {
		dur = defaultSTS
	}
	dur = clampSTS(dur)
	key, err := h.store.GetKeyByAccessID(r.Context(), req.AccessKeyID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if key == nil || key.AccountID != accountID || !auth.CheckPassword(req.SecretAccessKey, key.SecretHash) {
		httpx.Error(w, http.StatusUnauthorized, "Invalid access key credentials")
		return
	}
	if key.Status != "active" {
		httpx.Error(w, http.StatusForbidden, "Access key is disabled")
		return
	}
	app, err := h.store.GetApp(r.Context(), accountID, key.ApplicationID)
	if err != nil || app == nil {
		httpx.Error(w, http.StatusInternalServerError, "database error")
		return
	}
	if app.Status != "active" {
		httpx.Error(w, http.StatusForbidden, "Application is disabled")
		return
	}
	out := h.issueToken(r, key, req.SecretAccessKey, dur)
	if out == nil {
		httpx.Error(w, http.StatusInternalServerError, "token error")
		return
	}
	_ = h.store.TouchKey(r.Context(), key.ID)
	h.log.Record(r, user, "", key.AccessKeyID, "issue_sts_credentials", "success", "")
	httpx.JSON(w, http.StatusOK, out)
}

func (h *Handler) issueToken(r *http.Request, key *Key, secret string, dur int) map[string]any {
	if h.s3End != "" {
		creds, err := storage.SessionToken(r.Context(), h.s3End, h.s3Reg, key.AccessKeyID, secret, int32(dur))
		if err == nil {
			return stsJSON(creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken, creds.Expiration, dur, h.s3End, h.s3Reg)
		}
		slog.Warn("rgw sts unavailable, jwt fallback", "err", err)
	}
	exp := time.Now().Add(time.Duration(dur) * time.Second)
	token, err := signSTS(h.secret, key, exp)
	if err != nil {
		return nil
	}
	return stsJSON(key.AccessKeyID, secret, token, exp, dur, h.s3End, h.s3Reg)
}

func signSTS(secret string, key *Key, exp time.Time) (string, error) {
	now := time.Now()
	claims := stsClaims{
		Kind: stsKind, AccessKeyID: key.AccessKeyID, AccountID: key.AccountID, ApplicationID: key.ApplicationID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: strconv.FormatInt(key.ID, 10), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign sts: %w", err)
	}
	return s, nil
}

func parseSTS(secret, raw string) (*stsClaims, error) {
	tok, err := jwt.ParseWithClaims(raw, &stsClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected alg")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*stsClaims)
	if !ok || !tok.Valid || claims.Kind != stsKind {
		return nil, fmt.Errorf("invalid token")
	}
	return claims, nil
}

func stsJSON(keyID, secret, token string, exp time.Time, dur int, endpoint, region string) map[string]any {
	if region == "" {
		region = "us-east-1"
	}
	return map[string]any{
		"access_key_id":     keyID,
		"secret_access_key": secret,
		"session_token":     token,
		"expiration":        exp.UTC().Format(time.RFC3339),
		"duration_seconds":  dur,
		"endpoint_url":      endpoint,
		"region":            region,
	}
}
