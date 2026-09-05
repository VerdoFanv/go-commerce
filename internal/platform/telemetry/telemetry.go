// Package telemetry bootstraps OpenTelemetry tracing. When OTEL_ENABLED=false
// the setup is a no-op so local runs stay dependency-light; when enabled,
// spans export over OTLP gRPC to Jaeger/Tempo/any OTLP collector.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/verdofanv/golang-be/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// Provider wraps the tracer provider so callers can shut it down cleanly.
type Provider struct {
	tp      *sdktrace.TracerProvider
	enabled bool
}

func Setup(ctx context.Context, cfg config.Config, serviceName string) (*Provider, error) {
	if !cfg.OTELEnabled {
		slog.Info("otel disabled, tracing is a no-op")
		return &Provider{enabled: false}, nil
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTelEndpoint),
		otlptracegrpc.WithInsecure(), // dev/compose; terminate TLS at the collector in prod
	)
	if err != nil {
		return nil, fmt.Errorf("otel exporter: %w", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	// TraceContext + Baggage: the W3C standard understood by every modern gateway.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	slog.Info("otel tracing enabled", "endpoint", cfg.OTelEndpoint, "service", serviceName)
	return &Provider{tp: tp, enabled: true}, nil
}

func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || !p.enabled {
		return nil
	}
	return p.tp.Shutdown(ctx)
}
