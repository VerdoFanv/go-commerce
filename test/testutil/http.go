package testutil

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

// NewRouter returns a bare Fiber app for handler tests.
func NewRouter() *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Use(recover.New())
	return app
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

// DoJSON performs an HTTP request against a Fiber app and returns status + decoded envelope-ish map.
func DoJSON(t *testing.T, app *fiber.App, method, path string, body any, opts ...RequestOption) (int, map[string]any) {
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

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	var out map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("unmarshal response: %v\nbody=%s", err, string(raw))
		}
	}
	return resp.StatusCode, out
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
