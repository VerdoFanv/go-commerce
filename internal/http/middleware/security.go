package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders sets the OWASP baseline response headers on every response.
// When enableHSTS is true (TLS-terminated edge), Strict-Transport-Security is added.
func SecurityHeaders(enableHSTS bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "0")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		c.Header("Cache-Control", "no-store")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if enableHSTS {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}
