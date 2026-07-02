package telemetry

import (
	"context"
	"fmt"
	"os"
	"strings"

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

var sensitiveAttrKeys = []string{
	"api_key", "apikey", "token", "secret", "password", "passwd",
	"authorization", "auth", "credential", "access_key", "private_key",
}

func isSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, s := range sensitiveAttrKeys {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}

func Attr(key string, value string) attribute.KeyValue {
	if isSensitiveKey(key) {
		return attribute.String(key, "[REDACTED]")
	}
	return attribute.String(key, value)
}

func AttrInt(key string, value int) attribute.KeyValue {
	if isSensitiveKey(key) {
		return attribute.String(key, "[REDACTED]")
	}
	return attribute.Int(key, value)
}

func InitTracer(serviceName, endpoint string) (*sdktrace.TracerProvider, error) {
	if serviceName == "" {
		serviceName = os.Getenv("OTEL_SERVICE_NAME")
	}
	if serviceName == "" {
		serviceName = "noctifab"
	}

	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}

	exporterType := os.Getenv("OTEL_TRACES_EXPORTER")

	var exporter sdktrace.SpanExporter
	var err error

	switch {
	case exporterType == "stdout" || (endpoint == "" && exporterType != "otlp"):
		exporter, err = NewStdoutExporter()
	case endpoint != "":
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(endpoint),
		}
		if os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") != "false" {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exporter, err = otlptracehttp.New(context.Background(), opts...)
	default:
		exporter, err = NewStdoutExporter()
	}
	if err != nil {
		return nil, fmt.Errorf("telemetry: failed to create exporter: %w", err)
	}

	hostname, _ := os.Hostname()
	resAttrs := []attribute.KeyValue{
		attribute.String("service.name", serviceName),
		attribute.String("host.name", hostname),
	}
	if ra := os.Getenv("OTEL_RESOURCE_ATTRIBUTES"); ra != "" {
		for _, pair := range strings.Split(ra, ",") {
			pair = strings.TrimSpace(pair)
			if k, v, ok := strings.Cut(pair, "="); ok {
				resAttrs = append(resAttrs, attribute.String(strings.TrimSpace(k), strings.TrimSpace(v)))
			}
		}
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewWithAttributes(
			"https://opentelemetry.io/schema/1.21.0",
			resAttrs...,
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
