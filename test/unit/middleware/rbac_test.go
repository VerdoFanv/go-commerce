package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/http/middleware"
	"github.com/verdofanv/golang-be/test/testutil"
)

func TestRequireRole_AdminAllowed(t *testing.T) {
	app := testutil.NewRouter()
	secured := app.Group("/x", middleware.Auth("secret"), middleware.RequireRole(domain.RoleAdmin))
	secured.GET("", func(c *gin.Context) { c.Status(http.StatusOK) })

	token := testutil.SignTokenWithRole(t, "secret", 1, domain.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestRequireRole_UserDenied(t *testing.T) {
	app := testutil.NewRouter()
	secured := app.Group("/x", middleware.Auth("secret"), middleware.RequireRole(domain.RoleAdmin))
	secured.GET("", func(c *gin.Context) { c.Status(http.StatusOK) })

	token := testutil.SignTokenWithRole(t, "secret", 1, domain.RoleUser)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	require.Equal(t, http.StatusForbidden, rec.Code)
}

func TestRateLimit_NilLimiterPassesThrough(t *testing.T) {
	app := testutil.NewRouter()
	app.GET("/x", middleware.RateLimit(nil, 1, 0), func(c *gin.Context) { c.Status(http.StatusOK) })

	for range 5 {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}
}

func TestSecurityHeaders_Set(t *testing.T) {
	app := testutil.NewRouter()
	app.Use(middleware.SecurityHeaders())
	app.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	require.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", rec.Header().Get("X-Frame-Options"))
	require.NotEmpty(t, rec.Header().Get("Content-Security-Policy"))
}
