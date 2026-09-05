package middleware

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Timeout bounds every request with a context deadline. Repositories and
// services already honor ctx, so a slow downstream cancels the whole chain
// instead of tying up a Fiber worker.
func Timeout(d time.Duration) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(c.UserContext(), d)
		defer cancel()
		c.SetUserContext(ctx)
		return c.Next()
	}
}
