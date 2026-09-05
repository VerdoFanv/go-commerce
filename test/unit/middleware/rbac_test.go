package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/test/testutil"
)

func TestRequireRole_AdminAllowed(t *testing.T) {
	app := testutil.NewRouter()
	secured := app.Group("/x", middleware.Auth("secret"), middleware.RequireRole(domain.RoleAdmin))
	secured.Get("", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	token := testutil.SignTokenWithRole(t, "secret", 1, domain.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRequireRole_UserDenied(t *testing.T) {
	app := testutil.NewRouter()
	secured := app.Group("/x", middleware.Auth("secret"), middleware.RequireRole(domain.RoleAdmin))
	secured.Get("", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	token := testutil.SignTokenWithRole(t, "secret", 1, domain.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)
}

func TestRateLimit_NilLimiterPassesThrough(t *testing.T) {
	app := testutil.NewRouter()
	app.Get("/x", middleware.RateLimit(nil, 1, 0), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	// Even 5 requests with max=1 must pass: nil limiter = disabled.
	for range 5 {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		resp, err := app.Test(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode)
		_ = resp.Body.Close()
	}
}

func TestSecurityHeaders_Set(t *testing.T) {
	app := testutil.NewRouter()
	app.Use(middleware.SecurityHeaders())
	app.Get("/x", func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", resp.Header.Get("X-Frame-Options"))
	require.NotEmpty(t, resp.Header.Get("Content-Security-Policy"))
}
