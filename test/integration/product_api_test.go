package integration_test

import (
	"strconv"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/internal/auth"
	"github.com/verdofanv/golang-be/internal/middleware"
	"github.com/verdofanv/golang-be/internal/product"
	"github.com/verdofanv/golang-be/test/mocks"
	"github.com/verdofanv/golang-be/test/testutil"
)

func setupProductAPI() (*fiber.App, string) {
	cfg := testutil.Config()

	authRepo := mocks.NewAuthRepository()
	authSvc := auth.NewService(authRepo, cfg)
	authHandler := auth.NewHandler(authSvc)

	productRepo := mocks.NewProductRepository()
	productSvc := product.NewService(productRepo, nil, nil, cfg)
	productHandler := product.NewHandler(productSvc)

	app := testutil.NewRouter()
	api := app.Group("/api/v1", middleware.APIKey(cfg.APIKey))
	authHandler.RegisterRoutes(api, cfg.JWTSecret)
	productHandler.RegisterRoutes(api, cfg.JWTSecret)
	return app, cfg.APIKey
}

func registerAndLogin(t *testing.T, app *fiber.App, apiKey string) string {
	t.Helper()

	_, body := testutil.DoJSON(t, app, "POST", "/api/v1/authentication/register", map[string]any{
		"name": "Andi", "email": "andi@example.com", "password": "secret1",
	}, testutil.WithAPIKey(apiKey))

	data := testutil.DecodeData[map[string]any](t, body)
	tokens := data["tokens"].(map[string]any)
	return tokens["accessToken"].(string)
}

func TestProductAPI_CRUD(t *testing.T) {
	app, apiKey := setupProductAPI()
	access := registerAndLogin(t, app, apiKey)

	status, body := testutil.DoJSON(t, app, "POST", "/api/v1/products", map[string]any{
		"name": "Kopi Susu", "description": "Iced", "price": 28000, "stock": 10,
	}, testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, 201, status)

	created := testutil.DecodeData[map[string]any](t, body)
	id := int(created["id"].(float64))
	require.Equal(t, "Kopi Susu", created["name"])

	status, body = testutil.DoJSON(t, app, "GET", "/api/v1/products", nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, 200, status)
	list := testutil.DecodeData[[]any](t, body)
	require.Len(t, list, 1)

	idPath := "/api/v1/products/" + strconv.Itoa(id)

	status, body = testutil.DoJSON(t, app, "GET", idPath, nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, 200, status)
	got := testutil.DecodeData[map[string]any](t, body)
	require.Equal(t, "Kopi Susu", got["name"])

	status, body = testutil.DoJSON(t, app, "PUT", idPath, map[string]any{
		"name": "Kopi Hot", "price": 25000,
	}, testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, 200, status)
	updated := testutil.DecodeData[map[string]any](t, body)
	require.Equal(t, "Kopi Hot", updated["name"])

	status, _ = testutil.DoJSON(t, app, "DELETE", idPath, nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, 200, status)

	status, _ = testutil.DoJSON(t, app, "GET", idPath, nil,
		testutil.WithAPIKey(apiKey), testutil.WithBearer(access))
	require.Equal(t, 404, status)
}

func TestProductAPI_UnauthorizedWithoutToken(t *testing.T) {
	app, apiKey := setupProductAPI()

	status, _ := testutil.DoJSON(t, app, "GET", "/api/v1/products", nil,
		testutil.WithAPIKey(apiKey))
	require.Equal(t, 401, status)
}
