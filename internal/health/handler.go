// Package health implements Kubernetes-style probes: /health/live answers
// "is the process alive" while /health/ready checks every dependency and
// answers "should the load balancer send us traffic".
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

// ready pings every dependency in parallel; any failure → 503 with detail.
func (h *Handler) ready(c *gin.Context) {
	results := make(map[string]string, len(h.checkers))
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, checker := range h.checkers {
		wg.Add(1)
		go func(ch Checker) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
			defer cancel()

			status := "ok"
			if err := ch.Ping(ctx); err != nil {
				status = "down: " + err.Error()
			}
			mu.Lock()
			results[ch.Name()] = status
			mu.Unlock()
		}(checker)
	}
	wg.Wait()

	for _, status := range results {
		if status != "ok" {
			response.FailCode(c, http.StatusServiceUnavailable,
				domain.ErrUnavailable.Error(), domain.ErrorCode(domain.ErrUnavailable))
			return
		}
	}
	response.OK(c, "ready", gin.H{"checks": results})
}
