package middleware

import (
	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// fiberCarrier adapts Fiber's header map to OpenTelemetry's TextMapCarrier.
type fiberCarrier map[string][]string

func (fc fiberCarrier) Get(key string) string {
	values := fc[key]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func (fc fiberCarrier) Set(key, value string) { fc[key] = []string{value} }

func (fc fiberCarrier) Keys() []string {
	keys := make([]string, 0, len(fc))
	for k := range fc {
		keys = append(keys, k)
	}
	return keys
}

// Tracing starts a server span per request, extracting any incoming W3C
// TraceContext so downstream traces join the same distributed trace.
// With no TracerProvider configured (OTEL_ENABLED=false) spans are no-ops.
func Tracing(serviceName string) fiber.Handler {
	tracer := otel.Tracer(serviceName)
	propagator := otel.GetTextMapPropagator()
	if propagator == nil {
		propagator = propagation.TraceContext{}
	}

	return func(c *fiber.Ctx) error {
		ctx := propagator.Extract(c.UserContext(), fiberCarrier(c.GetReqHeaders()))
		spanName := c.Method() + " " + c.Route().Path

		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				semconv.HTTPRequestMethodKey.String(c.Method()),
				semconv.URLPath(c.Path()),
			),
		)
		defer span.End()

		c.SetUserContext(ctx)
		err := c.Next()

		status := c.Response().StatusCode()
		span.SetAttributes(
			attribute.Int("http.response.status_code", status),
			attribute.String("http.route", c.Route().Path),
		)
		if status >= 500 {
			span.SetStatus(codes.Error, "server error")
		}
		return err
	}
}
