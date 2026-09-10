// Package health implements Kubernetes-style probes: /health/live answers
// "is the process alive" while /health/ready checks dependencies and answers
// "should the load balancer send us traffic".
//
// Critical checkers (Postgres, Redis) failing → 503.
// Optional checkers (Mongo, Typesense) failing → 200 degraded with detail —
// so search/audit outages do not drain the whole API from the Service.
package health

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/verdofanv/golang-be/internal/domain"
	"github.com/verdofanv/golang-be/pkg/response"
)

// Checker is any dependency that can verify its own connectivity.
type Checker interface {
	Name() string
	Ping(ctx context.Context) error
	// Critical=true → readiness 503 when Ping fails.
	Critical() bool
}

type Handler struct {
	checkers []Checker
	timeout  time.Duration
}

func NewHandler(checkers ...Checker) *Handler {
	return &Handler{checkers: checkers, timeout: 2 * time.Second}
}

func (h *Handler) RegisterRoutes(r gin.IRouter) {
	r.GET("/health/live", h.live)
	r.GET("/health/ready", h.ready)
	// Backward-compatible alias: /health == readiness.
	r.GET("/health", h.ready)
}

// live only proves the server responds — no dependency checks.
func (h *Handler) live(c *gin.Context) {
	response.OK(c, "alive", gin.H{"status": "up"})
}

// ready pings every dependency in parallel.
// Any critical failure → 503. Optional-only failures → 200 "degraded".
func (h *Handler) ready(c *gin.Context) {
	type result struct {
		name     string
		status   string
		critical bool
	}
	results := make([]result, len(h.checkers))
	var wg sync.WaitGroup

	for i, checker := range h.checkers {
		wg.Add(1)
		go func(idx int, ch Checker) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
			defer cancel()

			status := "ok"
			if err := ch.Ping(ctx); err != nil {
				status = "down: " + err.Error()
			}
			results[idx] = result{name: ch.Name(), status: status, critical: ch.Critical()}
		}(i, checker)
	}
	wg.Wait()

	checks := make(map[string]string, len(results))
	criticalDown := false
	optionalDown := false
	for _, r := range results {
		checks[r.name] = r.status
		if r.status != "ok" {
			if r.critical {
				criticalDown = true
			} else {
				optionalDown = true
			}
		}
	}

	if criticalDown {
		c.JSON(http.StatusServiceUnavailable, response.Envelope{
			Success:   false,
			Message:   domain.ErrUnavailable.Error(),
			ErrorCode: domain.ErrorCode(domain.ErrUnavailable),
			Data:      gin.H{"checks": checks, "mode": "unavailable"},
		})
		return
	}

	payload := gin.H{"checks": checks}
	if optionalDown {
		payload["mode"] = "degraded"
		response.OK(c, "degraded", payload)
		return
	}
	response.OK(c, "ready", payload)
}
