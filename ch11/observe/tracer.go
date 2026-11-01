package observe

import (
	"context"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// InitTracer configures OTLP/HTTP exporting when OTEL_EXPORTER_OTLP_ENDPOINT is set.
// Without it the SDK keeps spans in-process, which makes local development dependency-free.
func InitTracer(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	resource, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, err
	}
	opts := []trace.TracerProviderOption{trace.WithResource(resource)}
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		exporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
		if err != nil {
			return nil, err
		}
		opts = append(opts, trace.WithBatcher(exporter))
	}
	tp := trace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}
