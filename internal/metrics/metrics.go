// Package metrics defines the Prometheus registry and Gin middleware that
// records RED (Rate, Errors, Duration) for every HTTP request, plus custom
// business counters for the event pipeline.
package metrics

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
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

	OutboxPending = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "golangbe",
		Subsystem: "outbox",
		Name:      "pending",
		Help:      "Unpublished transactional outbox rows (lag signal).",
	})

	WebSocketConnections = promauto.NewGauge(prometheus.GaugeOpts{
		Namespace: "golangbe",
		Subsystem: "websocket",
		Name:      "active_connections",
		Help:      "Currently connected WebSocket clients.",
	})
)

// Middleware records RED metrics per route template (FullPath).
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(c.Writer.Status())

		HTTPRequestsTotal.WithLabelValues(c.Request.Method, route, status).Inc()
		HTTPRequestDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
	}
}
