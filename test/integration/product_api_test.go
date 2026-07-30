package integration_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/auth"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/internal/product"
	"github.com/verdofanv/golang-be/test/mocks"
	"github.com/verdofanv/golang-be/test/testutil"
)

func setupProductAPI() (http.Handler, string) {
	cfg := testutil.Config()

	authRepo := mocks.NewAuthRepository()
	authSvc := auth.NewService(authRepo, cfg)
	authHandler := auth.NewHandler(authSvc)

	productRepo := mocks.NewProductRepository()
	productSvc := product.NewService(productRepo, nil, nil, cfg)
	productHandler := product.NewHandler(productSvc)

	r := testutil.NewRouter()
	api := r.Group("/api/v1", middleware.APIKey(cfg.APIKey))
	authHandler.RegisterRoutes(api, cfg.JWTSecret)
	productHandler.RegisterRoutes(api, cfg.JWTSecret)
	return r, cfg.APIKey
}

func registerAndLogin(t *testing.T, r http.Handler, apiKey string) string {
	t.Helper()

	_, body := testutil.DoJSON(t, r, http.MethodPost, "/api/v1/authentication/register", map[string]any{
		"name": "Andi", "email": "andi@example.com", "password": "secret1",
	}, testutil.WithAPIKey(apiKey))

	data := testutil.DecodeData[map[string]any](t, body)
	tokens := data["tokens"].(map[string]any)
	return tokens["accessToken"].(string)
}

func TestProductAPI_CRUD(t *testing.T) {
	r, apiKey := setupProductAPI()
	access := registerAndLogin(t, r, apiKey)

	status, body := testutil.DoJSON(t, r, http.MethodPost, "/api/v1/products", map[string]any{
		"name": "Kopi Susu", "description": "Iced", "price": 28000, "stock": 10,
	}, testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, http.StatusCreated, status)

	created := testutil.DecodeData[map[string]any](t, body)
	id := int(created["id"].(float64))
	require.Equal(t, "Kopi Susu", created["name"])

	status, body = testutil.DoJSON(t, r, http.MethodGet, "/api/v1/products", nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, http.StatusOK, status)
	list := testutil.DecodeData[[]any](t, body)
	require.Len(t, list, 1)

	idPath := "/api/v1/products/" + strconv.Itoa(id)

	status, body = testutil.DoJSON(t, r, http.MethodGet, idPath, nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, http.StatusOK, status)
	got := testutil.DecodeData[map[string]any](t, body)
	require.Equal(t, "Kopi Susu", got["name"])

	status, body = testutil.DoJSON(t, r, http.MethodPut, idPath, map[string]any{
		"name": "Kopi Hot", "price": 25000,
	}, testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, http.StatusOK, status)
	updated := testutil.DecodeData[map[string]any](t, body)
	require.Equal(t, "Kopi Hot", updated["name"])

	status, _ = testutil.DoJSON(t, r, http.MethodDelete, idPath, nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, http.StatusOK, status)

	status, _ = testutil.DoJSON(t, r, http.MethodGet, idPath, nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, http.StatusNotFound, status)
}

func TestProductAPI_UnauthorizedWithoutToken(t *testing.T) {
	r, apiKey := setupProductAPI()

	status, _ := testutil.DoJSON(t, r, http.MethodGet, "/api/v1/products", nil,
		testutil.WithAPIKey(apiKey))
	require.Equal(t, http.StatusUnauthorized, status)
}
