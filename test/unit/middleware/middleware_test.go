package middleware_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/test/testutil"
)

func doReq(engine *gin.Engine, method, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func TestAPIKey_Missing(t *testing.T) {
	app := testutil.NewRouter()
	app.GET("/x", middleware.APIKey("secret"), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := doReq(app, http.MethodGet, "/x", nil)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKey_Valid(t *testing.T) {
	app := testutil.NewRouter()
	app.GET("/x", middleware.APIKey("secret"), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := doReq(app, http.MethodGet, "/x", map[string]string{"apikey": "secret"})
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAPIKey_XAPIKeyHeader(t *testing.T) {
	app := testutil.NewRouter()
	app.GET("/x", middleware.APIKey("secret"), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := doReq(app, http.MethodGet, "/x", map[string]string{"X-API-Key": "secret"})
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAuth_MissingBearer(t *testing.T) {
	app := testutil.NewRouter()
	app.GET("/x", middleware.Auth("secret"), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := doReq(app, http.MethodGet, "/x", nil)
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuth_ValidAccessToken(t *testing.T) {
	secret := "secret"
	token := testutil.SignAccessToken(t, secret, 42)

	app := testutil.NewRouter()
	app.GET("/x", middleware.Auth(secret), func(c *gin.Context) {
		id, ok := middleware.UserID(c)
		require.True(t, ok)
		require.Equal(t, uint(42), id)
		c.Status(http.StatusOK)
	})

	rec := doReq(app, http.MethodGet, "/x", map[string]string{"Authorization": "Bearer " + token})
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAuth_RefreshTokenRejected(t *testing.T) {
	secret := "secret"
	token := testutil.SignRefreshToken(t, secret, 42)

	app := testutil.NewRouter()
	app.GET("/x", middleware.Auth(secret), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := doReq(app, http.MethodGet, "/x", map[string]string{"Authorization": "Bearer " + token})
	require.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAuth_ExpiredToken(t *testing.T) {
	secret := "secret"
	token := testutil.SignExpiredToken(t, secret, 42, "access")

	app := testutil.NewRouter()
	app.GET("/x", middleware.Auth(secret), func(c *gin.Context) { c.Status(http.StatusOK) })

	rec := doReq(app, http.MethodGet, "/x", map[string]string{"Authorization": "Bearer " + token})
	require.Equal(t, http.StatusUnauthorized, rec.Code)
	raw, err := io.ReadAll(rec.Body)
	require.NoError(t, err)
	require.Contains(t, string(raw), "Token expired")
}
