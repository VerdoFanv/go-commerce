// Package metrics defines the Prometheus registry and the Fiber middleware that
// records RED (Rate, Errors, Duration) for every HTTP request, plus custom
// business counters for the event pipeline.
package metrics

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	HTTPRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "golangbe",
		Subsystem: "http",
		Name:      "requests_total",
		Help:      "Total HTTP requests partitioned by method, route and status.",
	}, []string{"method", "route", "status"})

	HTTPRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "golangbe",
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "HTTP request latency distribution.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})

	EventsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "golangbe",
		Subsystem: "events",
		Name:      "published_total",
		Help:      "Domain events published to Kafka.",
	}, []string{"type"})

	EventsConsumed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "golangbe",
		Subsystem: "events",
		Name:      "consumed_total",
		Help:      "Domain events consumed from Kafka.",
	}, []string{"type", "consumer"})

	EventsDeadLettered = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "golangbe",
		Subsystem: "events",
		Name:      "dead_lettered_total",
		Help:      "Events sent to the DLQ after exhausting retries.",
	})

	WebSocketConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "golangbe",
		Subsystem: "websocket",
		Name:      "active_connections",
		Help:      "Currently connected WebSocket clients.",
	})
)

// Middleware returns a Fiber handler recording RED metrics per route template
// (Route().Path, not raw path — avoids cardinality explosion from path params).
func Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()

		route := c.Route().Path
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Response().StatusCode())

		HTTPRequestsTotal.WithLabelValues(c.Method(), route, status).Inc()
		HTTPRequestDuration.WithLabelValues(c.Method(), route).Observe(time.Since(start).Seconds())
		return err
	}
}
