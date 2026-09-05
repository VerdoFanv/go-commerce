package middleware

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/pkg/response"
)

// RequireRole enforces RBAC after Auth has populated the role local.
// Usage: adminOnly := middleware.RequireRole(domain.RoleAdmin)
func RequireRole(roles ...string) fiber.Handler {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(c *fiber.Ctx) error {
		role, ok := UserRole(c)
		if !ok {
			return response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
		}
		if _, ok := allowed[role]; !ok {
			return response.FailCode(c, http.StatusForbidden, domain.ErrForbidden.Error(), domain.ErrorCode(domain.ErrForbidden))
		}
		return c.Next()
	}
}
