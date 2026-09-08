package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/pkg/response"
)

// RequireRole enforces RBAC after Auth has populated the role in context.
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(c *gin.Context) {
		role, ok := UserRole(c)
		if !ok {
			response.FailCode(c, http.StatusUnauthorized, domain.ErrUnauthorized.Error(), domain.ErrorCode(domain.ErrUnauthorized))
			c.Abort()
			return
		}
		if _, ok := allowed[role]; !ok {
			response.FailCode(c, http.StatusForbidden, domain.ErrForbidden.Error(), domain.ErrorCode(domain.ErrForbidden))
			c.Abort()
			return
		}
		c.Next()
	}
}
