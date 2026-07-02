package telemetry

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func init() {
	tracer = trace.NewNoopTracerProvider().Tracer("noctifab")
}

func InitTracer(serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
	var exporter sdktrace.SpanExporter
	var err error

	if endpoint == "" {
		exporter, err = NewStdoutExporter()
	} else {
		exporter, err = otlptracehttp.New(context.Background(),
			otlptracehttp.WithEndpoint(endpoint),
			otlptracehttp.WithInsecure(),
		)
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: failed to create exporter: %w", err)
	}

	hostname, _ := os.Hostname()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			"https://opentelemetry.io/schema/1.21.0",
			attribute.String("service.name", serviceName),
			attribute.String("host.name", hostname),
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(tp)
	tracer = tp.Tracer(serviceName)
	return tp, nil
}

func Tracer() trace.Tracer {
	return tracer
}
