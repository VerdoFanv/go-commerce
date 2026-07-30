package integration_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/auth"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/test/mocks"
	"github.com/verdofanv/golang-be/test/testutil"
)

func setupAuthAPI() (http.Handler, *mocks.AuthRepository) {
	cfg := testutil.Config()
	repo := mocks.NewAuthRepository()
	svc := auth.NewService(repo, cfg)
	h := auth.NewHandler(svc)

	r := testutil.NewRouter()
	api := r.Group("/api/v1", middleware.APIKey(cfg.APIKey))
	h.RegisterRoutes(api, cfg.JWTSecret)
	return r, repo
}

func TestAuthAPI_RegisterLoginMe(t *testing.T) {
	r, _ := setupAuthAPI()
	cfg := testutil.Config()

	status, body := testutil.DoJSON(t, r, http.MethodPost, "/api/v1/authentication/register", map[string]any{
		"name": "Andi", "email": "andi@example.com", "password": "secret1",
	}, testutil.WithAPIKey(cfg.APIKey))
	require.Equal(t, http.StatusCreated, status)
	require.Equal(t, true, body["success"])

	status, body = testutil.DoJSON(t, r, http.MethodPost, "/api/v1/authentication/login", map[string]any{
		"email": "andi@example.com", "password": "secret1",
	}, testutil.WithAPIKey(cfg.APIKey))
	require.Equal(t, http.StatusOK, status)

	data := testutil.DecodeData[map[string]any](t, body)
	tokens := data["tokens"].(map[string]any)
	access := tokens["accessToken"].(string)
	require.NotEmpty(t, access)

	status, body = testutil.DoJSON(t, r, http.MethodGet, "/api/v1/authentication/me", nil,
		testutil.WithAPIKey(cfg.APIKey),
		testutil.WithBearer(access),
	)
	require.Equal(t, http.StatusOK, status)
	me := testutil.DecodeData[map[string]any](t, body)
	require.Equal(t, "andi@example.com", me["email"])
}

func TestAuthAPI_RegisterValidation(t *testing.T) {
	r, _ := setupAuthAPI()
	cfg := testutil.Config()

	status, body := testutil.DoJSON(t, r, http.MethodPost, "/api/v1/authentication/register", map[string]any{
		"name": "Andi", "email": "not-an-email", "password": "123",
	}, testutil.WithAPIKey(cfg.APIKey))
	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, false, body["success"])
}

func TestAuthAPI_RequiresAPIKey(t *testing.T) {
	r, _ := setupAuthAPI()

	status, body := testutil.DoJSON(t, r, http.MethodPost, "/api/v1/authentication/login", map[string]any{
		"email": "a@b.com", "password": "secret1",
	})
	require.Equal(t, http.StatusUnauthorized, status)
	require.Equal(t, domain.ErrInvalidAPIKey.Error(), body["message"])
}

func TestAuthAPI_RefreshToken(t *testing.T) {
	r, _ := setupAuthAPI()
	cfg := testutil.Config()

	_, body := testutil.DoJSON(t, r, http.MethodPost, "/api/v1/authentication/register", map[string]any{
		"name": "Andi", "email": "andi@example.com", "password": "secret1",
	}, testutil.WithAPIKey(cfg.APIKey))

	data := testutil.DecodeData[map[string]any](t, body)
	tokens := data["tokens"].(map[string]any)
	refresh := tokens["refreshToken"].(string)

	status, body := testutil.DoJSON(t, r, http.MethodPost, "/api/v1/authentication/refresh-token", map[string]any{
		"refreshToken": refresh,
	}, testutil.WithAPIKey(cfg.APIKey))
	require.Equal(t, http.StatusOK, status)

	refreshed := testutil.DecodeData[map[string]any](t, body)
	require.NotEmpty(t, refreshed["accessToken"])
	require.NotEmpty(t, refreshed["refreshToken"])
}
