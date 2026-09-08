package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

type httpHeaderMap map[string][]string

func (h httpHeaderMap) Get(key string) string {
	values := h[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (h httpHeaderMap) Set(key, value string) { h[key] = []string{value} }

func (h httpHeaderMap) Keys() []string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	return keys
}

// Tracing starts a server span per request using W3C TraceContext.
func Tracing(serviceName string) gin.HandlerFunc {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()
	if propagator == nil {
		propagator = propagation.TraceContext{}
	}

	return func(c *gin.Context) {
		carrier := httpHeaderMap(c.Request.Header)
		ctx := propagator.Extract(c.Request.Context(), carrier)

		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		spanName := c.Request.Method + " " + route

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(c.Request.Method),
				semconv.URLPath(c.Request.URL.Path),
			),
		)
		defer span.End()

		c.Request = c.Request.WithContext(ctx)
		c.Next()

		status := c.Writer.Status()
		span.SetAttributes(
			attribute.Int("http.response.status_code", status),
			attribute.String("http.route", route),
		)
		if status >= 500 {
			span.SetStatus(codes.Error, "server error")
		}
	}
}
