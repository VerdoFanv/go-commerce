package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// NewRouter returns a bare gin engine for handler tests.
func NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	return r
}

type RequestOption func(*http.Request)

func WithHeader(key, value string) RequestOption {
	return func(r *http.Request) {
		r.Header.Set(key, value)
	}
}

func WithJSON(body any) RequestOption {
	return func(r *http.Request) {
		r.Header.Set("Content-Type", "application/json")
	}
}

func WithBearer(token string) RequestOption {
	return WithHeader("Authorization", "Bearer "+token)
}

func WithAPIKey(key string) RequestOption {
	return WithHeader("apikey", key)
}

// DoJSON performs an HTTP request against a gin engine and returns status + decoded envelope-ish map.
func DoJSON(t *testing.T, r http.Handler, method, path string, body any, opts ...RequestOption) (int, map[string]any) {
	t.Helper()

	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(b)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, opt := range opts {
		opt(req)
	}

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out map[string]any
	if w.Body.Len() > 0 {
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal response: %v\nbody=%s", err, w.Body.String())
		}
	}
	return w.Code, out
}

func DecodeData[T any](t *testing.T, envelope map[string]any) T {
	t.Helper()
	raw, err := json.Marshal(envelope["data"])
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	return out
}
