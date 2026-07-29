package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/pkg/response"
)

const (
	ContextUserIDKey = "userID"
)

type Claims struct {
	UserID uint   `json:"userId"`
	Type   string `json:"type"`
	jwt.RegisteredClaims
}

func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"duration", time.Since(start).String(),
		)
	}
}

func APIKey(expected string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if expected == "" {
			c.Next()
			return
		}
		key := c.GetHeader("apikey")
		if key == "" {
			key = c.GetHeader("X-API-Key")
		}
		if key != expected {
			response.Fail(c, http.StatusUnauthorized, domain.ErrInvalidAPIKey.Error())
			c.Abort()
			return
		}
		c.Next()
	}
}

func Auth(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			return []byte(secret), nil
		})
		if err != nil || token == nil || !token.Valid {
			msg := domain.ErrUnauthorized.Error()
			if err != nil && strings.Contains(err.Error(), "token is expired") {
				msg = domain.ErrTokenExpired.Error()
			}
			response.Fail(c, http.StatusUnauthorized, msg)
			c.Abort()
			return
		}
		if claims.Type != "" && claims.Type != "access" {
			response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
			c.Abort()
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Next()
	}
}

func UserID(c *gin.Context) (uint, bool) {
	v, ok := c.Get(ContextUserIDKey)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}
