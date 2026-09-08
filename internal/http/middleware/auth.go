package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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

// Auth validates the Bearer token and stores UserID + Role in gin context.
func Auth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			unauthorized(c, domain.ErrUnauthorized)
			c.Abort()
			return
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
				unauthorized(c, domain.ErrTokenExpired)
				c.Abort()
				return
			}
			unauthorized(c, domain.ErrUnauthorized)
			c.Abort()
			return
		}
		if claims.Type != "" && claims.Type != "access" {
			unauthorized(c, domain.ErrUnauthorized)
			c.Abort()
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Set(ContextUserRoleKey, claims.Role)
		c.Next()
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

func unauthorized(c *gin.Context, err error) {
	response.FailCode(c, http.StatusUnauthorized, err.Error(), domain.ErrorCode(err))
}

// UserID extracts the authenticated user ID stored by Auth.
func UserID(c *gin.Context) (uint, bool) {
	v, ok := c.Get(ContextUserIDKey)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}

// UserRole extracts the authenticated role stored by Auth.
func UserRole(c *gin.Context) (string, bool) {
	v, ok := c.Get(ContextUserRoleKey)
	if !ok {
		return "", false
	}
	role, ok := v.(string)
	return role, ok
}
