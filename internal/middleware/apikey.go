package middleware

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/pkg/response"
)

// APIKey gates every route under /api with a shared service key — a first
// defense layer before any user-level JWT check.
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
			return response.FailCode(c, http.StatusUnauthorized, domain.ErrInvalidAPIKey.Error(), domain.ErrorCode(domain.ErrInvalidAPIKey))
		}
		return c.Next()
	}
}
