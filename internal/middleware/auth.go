package middleware

import (
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/pkg/response"
)

const (
	ContextUserIDKey   = "userID"
	ContextUserRoleKey = "userRole"
)

// Claims is the JWT payload. Role is embedded so authorization decisions
// never need a database round-trip per request.
type Claims struct {
	UserID uint   `json:"userId"`
	Role   string `json:"role"`
	Type   string `json:"type"`
	jwt.RegisteredClaims
}

// Auth validates the Bearer token and stores UserID + Role in request locals.
func Auth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return unauthorized(c, domain.ErrUnauthorized)
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, domain.ErrUnauthorized
			}
			return []byte(secret), nil
		})
		if err != nil || token == nil || !token.Valid {
			if err != nil && strings.Contains(err.Error(), "token is expired") {
				return unauthorized(c, domain.ErrTokenExpired)
			}
			return unauthorized(c, domain.ErrUnauthorized)
		}
		if claims.Type != "" && claims.Type != "access" {
			return unauthorized(c, domain.ErrUnauthorized)
		}

		c.Locals(ContextUserIDKey, claims.UserID)
		c.Locals(ContextUserRoleKey, claims.Role)
		return c.Next()
	}
}

// ParseToken validates a token outside the HTTP middleware chain (WebSocket upgrade).
func ParseToken(secret, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
	})
	if err != nil || token == nil || !token.Valid {
		return nil, domain.ErrUnauthorized
	}
	return claims, nil
}

func unauthorized(c *fiber.Ctx, err error) error {
	return response.FailCode(c, http.StatusUnauthorized, err.Error(), domain.ErrorCode(err))
}

// UserID extracts the authenticated user ID stored by Auth.
func UserID(c *fiber.Ctx) (uint, bool) {
	v := c.Locals(ContextUserIDKey)
	if v == nil {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}

// UserRole extracts the authenticated role stored by Auth.
func UserRole(c *fiber.Ctx) (string, bool) {
	v := c.Locals(ContextUserRoleKey)
	if v == nil {
		return "", false
	}
	role, ok := v.(string)
	return role, ok
}
