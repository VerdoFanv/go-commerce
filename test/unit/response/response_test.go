package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/verdofanv/golang-be/pkg/response"
	"github.com/verdofanv/golang-be/test/testutil"
)

func TestOK_Envelope(t *testing.T) {
	r := testutil.NewRouter()
	r.GET("/ok", func(c *gin.Context) {
		response.OK(c, "success", gin.H{"id": 1})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var env response.Envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.True(t, env.Success)
	require.Equal(t, "success", env.Message)
	require.NotNil(t, env.Data)
}

func TestFail_Envelope(t *testing.T) {
	r := testutil.NewRouter()
	r.GET("/fail", func(c *gin.Context) {
		response.Fail(c, http.StatusUnauthorized, "unauthorized")
	})

	req := httptest.NewRequest(http.MethodGet, "/fail", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)

	var env response.Envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env))
	require.False(t, env.Success)
	require.Equal(t, "unauthorized", env.Message)
}

func TestCreated_Status(t *testing.T) {
	r := testutil.NewRouter()
	r.POST("/created", func(c *gin.Context) {
		response.Created(c, "created", gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/created", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
}
