package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/pkg/response"
)

// APIKey gates every route under /api with a shared service key — a first
// defense layer before any user-level JWT check.
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
			response.FailCode(c, http.StatusUnauthorized, domain.ErrInvalidAPIKey.Error(), domain.ErrorCode(domain.ErrInvalidAPIKey))
			c.Abort()
			return
		}
		c.Next()
	}
}
