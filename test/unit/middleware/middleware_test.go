package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/test/testutil"
)

func TestAPIKey_Missing(t *testing.T) {
	app := testutil.NewRouter()
	app.Get("/x", middleware.APIKey("secret"), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAPIKey_Valid(t *testing.T) {
	app := testutil.NewRouter()
	app.Get("/x", middleware.APIKey("secret"), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("apikey", "secret")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAPIKey_XAPIKeyHeader(t *testing.T) {
	app := testutil.NewRouter()
	app.Get("/x", middleware.APIKey("secret"), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", "secret")
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAuth_MissingBearer(t *testing.T) {
	app := testutil.NewRouter()
	app.Get("/x", middleware.Auth("secret"), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAuth_ValidAccessToken(t *testing.T) {
	secret := "secret"
	token := testutil.SignAccessToken(t, secret, 42)

	app := testutil.NewRouter()
	app.Get("/x", middleware.Auth(secret), func(c *fiber.Ctx) error {
		id, ok := middleware.UserID(c)
		require.True(t, ok)
		require.Equal(t, uint(42), id)
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAuth_RefreshTokenRejected(t *testing.T) {
	secret := "secret"
	token := testutil.SignRefreshToken(t, secret, 42)

	app := testutil.NewRouter()
	app.Get("/x", middleware.Auth(secret), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestAuth_ExpiredToken(t *testing.T) {
	secret := "secret"
	token := testutil.SignExpiredToken(t, secret, 42, "access")

	app := testutil.NewRouter()
	app.Get("/x", middleware.Auth(secret), func(c *fiber.Ctx) error {
		return c.SendStatus(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(raw), "Token expired")
}
