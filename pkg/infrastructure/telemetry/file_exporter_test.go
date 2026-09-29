package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func TestFileExporter_Writer(t *testing.T) {
	var buf bytes.Buffer
	exp := NewWriterExporter(&buf)
	require.NotNil(t, exp)

	tp, err := InitTracerWithFile("test-service", "", "")
	require.NoError(t, err)
	defer func() { _ = tp.Shutdown(context.Background()) }()

	ctx, span := Tracer().Start(context.Background(), "test-file-span",
		trace.WithAttributes(
			attribute.String("env", "testing"),
			attribute.Int("attempt", 1),
		))
	span.End()
	require.NotNil(t, ctx)

	require.NoError(t, exp.Shutdown(context.Background()))
}

func TestFileExporter_AppendsToFile(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	exp, err := NewFileExporter(tracePath)
	require.NoError(t, err)
	require.NotNil(t, exp)

	tp, err := InitTracerWithFile("file-service", "", tracePath)
	require.NoError(t, err)

	ctx, span := Tracer().Start(context.Background(), "workflow-span",
		trace.WithAttributes(
			attribute.String("task.id", "task-101"),
			attribute.String("action", "test_execute"),
		))
	span.End()

	require.NoError(t, tp.ForceFlush(ctx))
	require.NoError(t, tp.Shutdown(ctx))
	require.NoError(t, exp.Shutdown(ctx))

	data, err := os.ReadFile(tracePath)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	require.NotEmpty(t, lines)

	var record map[string]any
	err = json.Unmarshal([]byte(lines[0]), &record)
	require.NoError(t, err)
	assert.NotEmpty(t, record["name"])
	assert.NotEmpty(t, record["trace_id"])
	assert.NotEmpty(t, record["span_id"])
}

func TestFileExporter_AppendPreservesExistingLines(t *testing.T) {
	tmpDir := t.TempDir()
	tracePath := filepath.Join(tmpDir, "traces.jsonl")

	// Pre-seed an existing trace line
	initialLine := `{"name":"prior-span","trace_id":"trace-1"}` + "\n"
	require.NoError(t, os.WriteFile(tracePath, []byte(initialLine), 0644))

	// Open with NewFileExporter
	exp, err := NewFileExporter(tracePath)
	require.NoError(t, err)

	tp, err := InitTracerWithFile("file-service", "", tracePath)
	require.NoError(t, err)

	ctx, span := Tracer().Start(context.Background(), "new-span")
	span.End()

	require.NoError(t, tp.ForceFlush(ctx))
	require.NoError(t, tp.Shutdown(ctx))
	require.NoError(t, exp.Shutdown(ctx))

	data, err := os.ReadFile(tracePath)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// Must contain both the pre-existing line and the newly appended line
	require.GreaterOrEqual(t, len(lines), 2)
	assert.Contains(t, lines[0], "prior-span")
	assert.Contains(t, string(data), "new-span")
}
