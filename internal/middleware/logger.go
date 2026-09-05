package middleware

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RequestLogger emits one structured line per request. The request ID comes
// from the requestid middleware (mounted earlier in the chain) so logs,
// traces, and client-visible X-Request-ID headers all correlate.
func RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		attrs := []any{
			"requestId", c.GetRespHeader(fiber.HeaderXRequestID),
			"method", c.Method(),
			"path", c.Path(),
			"route", c.Route().Path,
			"status", c.Response().StatusCode(),
			"ip", c.IP(),
			"duration", time.Since(start).String(),
		}
		if userID, ok := UserID(c); ok {
			attrs = append(attrs, "userId", userID)
		}

		status := c.Response().StatusCode()
		switch {
		case status >= 500:
			slog.Error("request", attrs...)
		case status >= 400:
			slog.Warn("request", attrs...)
		default:
			slog.Info("request", attrs...)
		}
		return err
	}
}
