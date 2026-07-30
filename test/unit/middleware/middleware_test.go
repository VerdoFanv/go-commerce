package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/test/testutil"
)

func TestAPIKey_Missing(t *testing.T) {
	r := testutil.NewRouter()
	r.GET("/x", middleware.APIKey("secret"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAPIKey_Valid(t *testing.T) {
	r := testutil.NewRouter()
	r.GET("/x", middleware.APIKey("secret"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("apikey", "secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKey_XAPIKeyHeader(t *testing.T) {
	r := testutil.NewRouter()
	r.GET("/x", middleware.APIKey("secret"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("X-API-Key", "secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAuth_MissingBearer(t *testing.T) {
	r := testutil.NewRouter()
	r.GET("/x", middleware.Auth("secret"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_ValidAccessToken(t *testing.T) {
	secret := "secret"
	token := testutil.SignAccessToken(t, secret, 42)

	r := testutil.NewRouter()
	r.GET("/x", middleware.Auth(secret), func(c *gin.Context) {
		id, ok := middleware.UserID(c)
		require.True(t, ok)
		require.Equal(t, uint(42), id)
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestAuth_RefreshTokenRejected(t *testing.T) {
	secret := "secret"
	token := testutil.SignRefreshToken(t, secret, 42)

	r := testutil.NewRouter()
	r.GET("/x", middleware.Auth(secret), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_ExpiredToken(t *testing.T) {
	secret := "secret"
	token := testutil.SignExpiredToken(t, secret, 42, "access")

	r := testutil.NewRouter()
	r.GET("/x", middleware.Auth(secret), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	require.Contains(t, w.Body.String(), "Token expired")
}
