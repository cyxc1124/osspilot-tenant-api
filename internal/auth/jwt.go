package auth

import (
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const PortalTenant = "tenant"

type Claims struct {
	UserID int64    `json:"user_id"`
	Portal string   `json:"portal"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

func IssueToken(secret string, ttl time.Duration, userID int64, role string) (string, error) {
	now := time.Now()
	roles := []string{}
	if role != "" {
		roles = []string{role}
	}
	claims := Claims{
		UserID: userID,
		Portal: PortalTenant,
		Roles:  roles,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(userID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return s, nil
}

func ParseToken(secret, raw string) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(raw, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected alg")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, fmt.Errorf("invalid token")
	}
	if claims.Portal != "" && claims.Portal != PortalTenant {
		return nil, fmt.Errorf("wrong portal")
	}
	return claims, nil
}
