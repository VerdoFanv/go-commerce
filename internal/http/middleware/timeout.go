package middleware

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
)

// Timeout bounds every request with a context deadline.
func Timeout(d time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), d)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}
