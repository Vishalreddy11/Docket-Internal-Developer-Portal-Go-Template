// Package otel sets up an OTLP HTTP trace exporter. If the configured endpoint
// is empty or unreachable, tracing is disabled and the rest of the app keeps
// running — observability must never block business code.
package otel

import (
	"context"
	"log/slog"
	"strings"

	"github.com/example/docket/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

type ShutdownFunc func(context.Context) error

func Init(ctx context.Context, cfg config.OTelConfig, log *slog.Logger) (ShutdownFunc, error) {
	if cfg.Endpoint == "" {
		log.Info("otel disabled (no endpoint configured)")
		return func(context.Context) error { return nil }, nil
	}

	endpoint := strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://")
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint)}
	if !strings.HasPrefix(cfg.Endpoint, "https://") {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return func(context.Context) error { return nil }, err
	}

	res, _ := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)),
	)

	sampler := sdktrace.AlwaysSample()
	if cfg.Sampler == "always_off" {
		sampler = sdktrace.NeverSample()
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	log.Info("otel tracer initialized", "endpoint", cfg.Endpoint, "service", cfg.ServiceName)
	return tp.Shutdown, nil
}
