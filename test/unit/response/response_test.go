package response_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/pkg/response"
	"github.com/verdofanv/golang-be/test/testutil"
)

func TestOK_Envelope(t *testing.T) {
	app := testutil.NewRouter()
	app.Get("/ok", func(c *fiber.Ctx) error {
		return response.OK(c, "success", fiber.Map{"id": 1})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusOK, resp.StatusCode)

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var env response.Envelope
	require.NoError(t, json.Unmarshal(raw, &env))
	require.True(t, env.Success)
	require.Equal(t, "success", env.Message)
	require.NotNil(t, env.Data)
}

func TestFail_Envelope(t *testing.T) {
	app := testutil.NewRouter()
	app.Get("/fail", func(c *fiber.Ctx) error {
		return response.Fail(c, http.StatusUnauthorized, "unauthorized")
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var env response.Envelope
	require.NoError(t, json.Unmarshal(raw, &env))
	require.False(t, env.Success)
	require.Equal(t, "unauthorized", env.Message)
}

func TestCreated_Status(t *testing.T) {
	app := testutil.NewRouter()
	app.Post("/created", func(c *fiber.Ctx) error {
		return response.Created(c, "created", fiber.Map{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/created", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	require.Equal(t, http.StatusCreated, resp.StatusCode)
}
