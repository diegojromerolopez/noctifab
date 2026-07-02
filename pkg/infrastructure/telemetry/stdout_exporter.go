package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type StdoutExporter struct {
	mu      sync.Mutex
	encoder *json.Encoder
}

func NewStdoutExporter() (*StdoutExporter, error) {
	return &StdoutExporter{
		encoder: json.NewEncoder(os.Stdout),
	}, nil
}

func (e *StdoutExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, span := range spans {
		record := map[string]any{
			"name":       span.Name(),
			"trace_id":   span.SpanContext().TraceID().String(),
			"span_id":    span.SpanContext().SpanID().String(),
			"start_time": span.StartTime().Format(time.RFC3339Nano),
			"end_time":   span.EndTime().Format(time.RFC3339Nano),
			"status":     span.Status().Code.String(),
			"attributes": span.Attributes(),
		}
		if err := e.encoder.Encode(record); err != nil {
			return fmt.Errorf("stdout exporter: encode: %w", err)
		}
	}
	return nil
}

func (e *StdoutExporter) Shutdown(ctx context.Context) error {
	return nil
}

var _ sdktrace.SpanExporter = (*StdoutExporter)(nil)
