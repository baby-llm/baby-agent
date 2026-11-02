package observe

import (
	"context"
	"net/url"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

// InitTracer configures OTLP/HTTP exporting when OTEL_EXPORTER_OTLP_ENDPOINT is set.
// Without it the SDK keeps spans in-process, which makes local development dependency-free.
func InitTracer(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(serviceName)))
	if err != nil {
		return nil, err
	}
	opts := []trace.TracerProviderOption{trace.WithResource(res)}
	if endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")); endpoint != "" {
		exporter, err := otlptracehttp.New(ctx, otlpOptions(endpoint)...)
		if err != nil {
			return nil, err
		}
		opts = append(opts, trace.WithBatcher(exporter))
	}
	tp := trace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

// otlpOptions accepts both the documented host:port form and a full URL such
// as http://localhost:4318. Passing a URL straight to WithEndpoint fails
// because the OTLP HTTP exporter expects only the host and port.
func otlpOptions(endpoint string) []otlptracehttp.Option {
	opts := []otlptracehttp.Option{otlptracehttp.WithInsecure()}
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(u.Host))
		if u.Path != "" && u.Path != "/" {
			opts = append(opts, otlptracehttp.WithURLPath(u.Path))
		}
		return opts
	}
	return append(opts, otlptracehttp.WithEndpoint(endpoint))
}
