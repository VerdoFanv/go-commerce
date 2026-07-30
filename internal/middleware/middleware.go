package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
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

func RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		slog.Info("request",
			"method", c.Method(),
			"path", c.Path(),
			"status", c.Response().StatusCode(),
			"duration", time.Since(start).String(),
		)
		return err
	}
}

func APIKey(expected string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if expected == "" {
			return c.Next()
		}
		key := c.Get("apikey")
		if key == "" {
			key = c.Get("X-API-Key")
		}
		if key != expected {
			return response.Fail(c, http.StatusUnauthorized, domain.ErrInvalidAPIKey.Error())
		}
		return c.Next()
	}
}

func Auth(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		header := c.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			return response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
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
			return response.Fail(c, http.StatusUnauthorized, msg)
		}
		if claims.Type != "" && claims.Type != "access" {
			return response.Fail(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error())
		}

		c.Locals(ContextUserIDKey, claims.UserID)
		return c.Next()
	}
}

func UserID(c *fiber.Ctx) (uint, bool) {
	v := c.Locals(ContextUserIDKey)
	if v == nil {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}
