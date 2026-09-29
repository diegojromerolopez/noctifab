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
	"go.opentelemetry.io/otel/trace/noop"
)

var tracer trace.Tracer

func init() {
	tracer = noop.NewTracerProvider().Tracer("noctifab")
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
	return InitTracerWithFile(serviceName, endpoint, "")
}

func InitTracerWithFile(serviceName, endpoint, traceFile string) (*sdktrace.TracerProvider, error) {
	if serviceName == "" {
		serviceName = os.Getenv("OTEL_SERVICE_NAME")
	}
	if serviceName == "" {
		serviceName = "noctifab"
	}

	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}

	if traceFile == "" {
		traceFile = os.Getenv("OTEL_TRACES_FILE")
	}

	exporterType := os.Getenv("OTEL_TRACES_EXPORTER")

	var batcherOpts []sdktrace.TracerProviderOption

	if traceFile != "" {
		fileExp, fErr := NewFileExporter(traceFile)
		if fErr != nil {
			return nil, fmt.Errorf("telemetry: failed to create file exporter: %w", fErr)
		}
		batcherOpts = append(batcherOpts, sdktrace.WithBatcher(fileExp))
	}

	if endpoint != "" {
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(endpoint),
		}
		if os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") != "false" {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		otlpExp, oErr := otlptracehttp.New(context.Background(), opts...)
		if oErr != nil {
			return nil, fmt.Errorf("telemetry: failed to create otlp exporter: %w", oErr)
		}
		batcherOpts = append(batcherOpts, sdktrace.WithBatcher(otlpExp))
	} else if len(batcherOpts) == 0 {
		stdoutExp, sErr := NewStdoutExporter()
		if sErr != nil {
			return nil, fmt.Errorf("telemetry: failed to create stdout exporter: %w", sErr)
		}
		batcherOpts = append(batcherOpts, sdktrace.WithBatcher(stdoutExp))
	} else if exporterType == "stdout" {
		stdoutExp, sErr := NewStdoutExporter()
		if sErr == nil {
			batcherOpts = append(batcherOpts, sdktrace.WithBatcher(stdoutExp))
		}
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

	providerOpts := append([]sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.NewWithAttributes(
			"https://opentelemetry.io/schema/1.21.0",
			resAttrs...,
		)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	}, batcherOpts...)

	tp := sdktrace.NewTracerProvider(providerOpts...)

	otel.SetTracerProvider(tp)
	tracer = tp.Tracer(serviceName)
	return tp, nil
}

func Tracer() trace.Tracer {
	return tracer
}
