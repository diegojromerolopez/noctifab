package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// FileExporter exports OpenTelemetry spans to a file formatted as JSON Lines.
type FileExporter struct {
	mu      sync.Mutex
	encoder *json.Encoder
	closer  io.Closer
}

// NewFileExporter creates a FileExporter that appends spans as JSON Lines to filePath.
func NewFileExporter(filePath string) (*FileExporter, error) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("file exporter: failed to create directory %q: %w", dir, err)
	}

	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("file exporter: failed to open file %q: %w", filePath, err)
	}

	return &FileExporter{
		encoder: json.NewEncoder(f),
		closer:  f,
	}, nil
}

// NewWriterExporter creates a FileExporter that writes to an arbitrary io.Writer.
func NewWriterExporter(w io.Writer) *FileExporter {
	var closer io.Closer
	if c, ok := w.(io.Closer); ok {
		closer = c
	}
	return &FileExporter{
		encoder: json.NewEncoder(w),
		closer:  closer,
	}
}

// ExportSpans encodes spans as JSONL records.
func (e *FileExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for _, span := range spans {
		attrs := make(map[string]any, len(span.Attributes()))
		for _, kv := range span.Attributes() {
			attrs[string(kv.Key)] = kv.Value.AsInterface()
		}

		record := map[string]any{
			"name":        span.Name(),
			"trace_id":    span.SpanContext().TraceID().String(),
			"span_id":     span.SpanContext().SpanID().String(),
			"start_time":  span.StartTime().Format(time.RFC3339Nano),
			"end_time":    span.EndTime().Format(time.RFC3339Nano),
			"duration_ms": span.EndTime().Sub(span.StartTime()).Milliseconds(),
			"status":      span.Status().Code.String(),
			"attributes":  attrs,
		}
		if err := e.encoder.Encode(record); err != nil {
			return fmt.Errorf("file exporter: encode: %w", err)
		}
	}
	return nil
}

// Shutdown flushes and closes the underlying file handle if open.
func (e *FileExporter) Shutdown(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closer != nil {
		return e.closer.Close()
	}
	return nil
}

var _ sdktrace.SpanExporter = (*FileExporter)(nil)
