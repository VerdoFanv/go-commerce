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
		claims, err := ParseAccessToken(secret, tokenStr)
		if err != nil {
			unauthorized(c, err)
			c.Abort()
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Set(ContextUserRoleKey, claims.Role)
		c.Next()
	}
}

// ParseAccessToken validates an access token (HMAC + type=access). Used by Auth and WebSocket.
func ParseAccessToken(secret, tokenStr string) (*Claims, error) {
	claims, err := parseToken(secret, tokenStr)
	if err != nil {
		return nil, err
	}
	if claims.Type != "" && claims.Type != "access" {
		return nil, domain.ErrUnauthorized
	}
	return claims, nil
}

// ParseToken is an alias kept for callers; enforces the same rules as HTTP Auth.
func ParseToken(secret, tokenStr string) (*Claims, error) {
	return ParseAccessToken(secret, tokenStr)
}

func parseToken(secret, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, domain.ErrUnauthorized
		}
		return []byte(secret), nil
	})
	if err != nil || token == nil || !token.Valid {
		if err != nil && strings.Contains(err.Error(), "token is expired") {
			return nil, domain.ErrTokenExpired
		}
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
