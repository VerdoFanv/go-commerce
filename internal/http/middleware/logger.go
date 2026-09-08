package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLogger emits one structured line per request.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		attrs := []any{
			"requestId", c.Writer.Header().Get("X-Request-ID"),
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"route", route,
			"status", c.Writer.Status(),
			"ip", c.ClientIP(),
			"duration", time.Since(start).String(),
		}
		if userID, ok := UserID(c); ok {
			attrs = append(attrs, "userId", userID)
		}

		status := c.Writer.Status()
		switch {
		case status >= 500:
			slog.Error("request", attrs...)
		case status >= 400:
			slog.Warn("request", attrs...)
		default:
			slog.Info("request", attrs...)
		}
	}
}
