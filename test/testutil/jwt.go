package testutil

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/verdofanv/golang-be/internal/middleware"
)

func SignToken(t *testing.T, secret string, userID uint, tokenType string, ttl time.Duration) string {
	t.Helper()

	claims := middleware.Claims{
		UserID: userID,
		Type:   tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func SignAccessToken(t *testing.T, secret string, userID uint) string {
	t.Helper()
	return SignToken(t, secret, userID, "access", 15*time.Minute)
}

// SignTokenWithRole signs an access token carrying an RBAC role claim.
func SignTokenWithRole(t *testing.T, secret string, userID uint, role string) string {
	t.Helper()

	claims := middleware.Claims{
		UserID: userID,
		Role:   role,
		Type:   "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func SignRefreshToken(t *testing.T, secret string, userID uint) string {
	t.Helper()
	return SignToken(t, secret, userID, "refresh", 24*time.Hour)
}

func SignExpiredToken(t *testing.T, secret string, userID uint, tokenType string) string {
	t.Helper()
	return SignToken(t, secret, userID, tokenType, -time.Minute)
}
