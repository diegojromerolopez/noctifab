package telemetry

import (
	"context"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TestTracer_NoopDefault(t *testing.T) {
	tracer := Tracer()
	if tracer == nil {
		t.Fatal("expected non-nil tracer even before InitTracer")
	}
	_, span := tracer.Start(context.Background(), "test-span")
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	span.End()
}

func TestTracer_SpanCreatedAndEnded(t *testing.T) {
	tp, err := InitTracer("test-service", "")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := Tracer()
	ctx, span := tracer.Start(context.Background(), "test-span",
		trace.WithAttributes(attribute.String("key", "value")))
	span.End()

	if ctx == nil {
		t.Error("expected non-nil context")
	}
}

func TestTracer_InitStdoutFallback(t *testing.T) {
	tp, err := InitTracer("test-service", "")
	if err != nil {
		t.Fatalf("InitTracer with empty endpoint should use stdout exporter: %v", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()
}

func TestTracer_MultipleSpans(t *testing.T) {
	tp, err := InitTracer("multi-test", "")
	if err != nil {
		t.Fatalf("InitTracer failed: %v", err)
	}
	defer func() { _ = tp.Shutdown(context.Background()) }()

	tracer := Tracer()
	for i := 0; i < 5; i++ {
		_, span := tracer.Start(context.Background(), "child-span")
		span.End()
	}
}
