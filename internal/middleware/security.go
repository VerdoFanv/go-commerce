package middleware

import "github.com/gofiber/fiber/v2"

// SecurityHeaders sets the OWASP baseline response headers on every response.
// Equivalent of helmet() in the Express world, kept dependency-free.
func SecurityHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Content-Type-Options", "nosniff")
		c.Set("X-Frame-Options", "DENY")
		c.Set("X-XSS-Protection", "0") // modern browsers: CSP supersedes this legacy header
		c.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		c.Set("Cache-Control", "no-store")
		c.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		return c.Next()
	}
}
