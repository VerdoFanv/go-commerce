package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/verdofanv/golang-be/internal/http/health"
)

type stubCheck struct {
	name     string
	critical bool
	err      error
}

func (s stubCheck) Name() string                 { return s.name }
func (s stubCheck) Critical() bool               { return s.critical }
func (s stubCheck) Ping(context.Context) error   { return s.err }

func TestReadyCriticalDown503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := health.NewHandler(
		stubCheck{name: "postgres", critical: true, err: errors.New("down")},
		stubCheck{name: "typesense", critical: false, err: nil},
	)
	r := gin.New()
	h.RegisterRoutes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestReadyOptionalDownDegraded200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := health.NewHandler(
		stubCheck{name: "postgres", critical: true, err: nil},
		stubCheck{name: "typesense", critical: false, err: errors.New("down")},
	)
	r := gin.New()
	h.RegisterRoutes(r)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !contains(w.Body.String(), "degraded") {
		t.Fatalf("want degraded message, body=%s", w.Body.String())
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		(func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		})())
}
