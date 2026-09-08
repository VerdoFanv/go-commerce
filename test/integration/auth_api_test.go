package integration_test

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/http/auth"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/http/middleware"
	"github.com/verdofanv/golang-be/test/mocks"
	"github.com/verdofanv/golang-be/test/testutil"
)

func setupAuthAPI() (*gin.Engine, *mocks.AuthRepository) {
	cfg := testutil.Config()
	repo := mocks.NewAuthRepository()
	svc := auth.NewService(repo, cfg)
	h := auth.NewHandler(svc)

	app := testutil.NewRouter()
	api := app.Group("/api/v1", middleware.APIKey(cfg.APIKey))
	h.RegisterRoutes(api, cfg.JWTSecret)
	return app, repo
}

func TestAuthAPI_RegisterLoginMe(t *testing.T) {
	app, _ := setupAuthAPI()
	cfg := testutil.Config()

	status, body := testutil.DoJSON(t, app, "POST", "/api/v1/authentication/register", map[string]any{
		"name": "Andi", "email": "andi@example.com", "password": "secret1",
	}, testutil.WithAPIKey(cfg.APIKey))
	require.Equal(t, 201, status)
	require.Equal(t, true, body["success"])

	status, body = testutil.DoJSON(t, app, "POST", "/api/v1/authentication/login", map[string]any{
		"email": "andi@example.com", "password": "secret1",
	}, testutil.WithAPIKey(cfg.APIKey))
	require.Equal(t, 200, status)

	data := testutil.DecodeData[map[string]any](t, body)
	tokens := data["tokens"].(map[string]any)
	access := tokens["accessToken"].(string)
	require.NotEmpty(t, access)

	status, body = testutil.DoJSON(t, app, "GET", "/api/v1/authentication/me", nil,
		testutil.WithAPIKey(cfg.APIKey),
		testutil.WithBearer(access),
	)
	require.Equal(t, 200, status)
	me := testutil.DecodeData[map[string]any](t, body)
	require.Equal(t, "andi@example.com", me["email"])
}

func TestAuthAPI_RegisterValidation(t *testing.T) {
	app, _ := setupAuthAPI()
	cfg := testutil.Config()

	status, body := testutil.DoJSON(t, app, "POST", "/api/v1/authentication/register", map[string]any{
		"name": "Andi", "email": "not-an-email", "password": "123",
	}, testutil.WithAPIKey(cfg.APIKey))
	require.Equal(t, 400, status)
	require.Equal(t, false, body["success"])
}

func TestAuthAPI_RequiresAPIKey(t *testing.T) {
	app, _ := setupAuthAPI()

	status, body := testutil.DoJSON(t, app, "POST", "/api/v1/authentication/login", map[string]any{
		"email": "a@b.com", "password": "secret1",
	})
	require.Equal(t, 401, status)
	require.Equal(t, domain.ErrInvalidAPIKey.Error(), body["message"])
}

func TestAuthAPI_RefreshToken(t *testing.T) {
	app, _ := setupAuthAPI()
	cfg := testutil.Config()

	_, body := testutil.DoJSON(t, app, "POST", "/api/v1/authentication/register", map[string]any{
		"name": "Andi", "email": "andi@example.com", "password": "secret1",
	}, testutil.WithAPIKey(cfg.APIKey))

	data := testutil.DecodeData[map[string]any](t, body)
	tokens := data["tokens"].(map[string]any)
	refresh := tokens["refreshToken"].(string)

	status, body := testutil.DoJSON(t, app, "POST", "/api/v1/authentication/refresh-token", map[string]any{
		"refreshToken": refresh,
	}, testutil.WithAPIKey(cfg.APIKey))
	require.Equal(t, 200, status)

	refreshed := testutil.DecodeData[map[string]any](t, body)
	require.NotEmpty(t, refreshed["accessToken"])
	require.NotEmpty(t, refreshed["refreshToken"])
}
