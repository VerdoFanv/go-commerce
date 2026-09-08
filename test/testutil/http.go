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

// NewRouter returns a bare Gin engine for handler tests.
func NewRouter() *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Recovery())
	return engine
}

type RequestOption func(*http.Request)

func WithHeader(key, value string) RequestOption {
	return func(r *http.Request) {
		r.Header.Set(key, value)
	}
}

func WithBearer(token string) RequestOption {
	return WithHeader("Authorization", "Bearer "+token)
}

func WithAPIKey(key string) RequestOption {
	return WithHeader("apikey", key)
}

// DoJSON performs an HTTP request against a Gin engine and returns status + decoded map.
func DoJSON(t *testing.T, engine *gin.Engine, method, path string, body any, opts ...RequestOption) (int, map[string]any) {
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

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	var out map[string]any
	raw := rec.Body.Bytes()
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal response: %v\nbody=%s", err, string(raw))
		}
	}
	return rec.Code, out
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
