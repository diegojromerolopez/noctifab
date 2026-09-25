package telemetry

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadRecentSpans_EmptyOrNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	spans, err := ReadRecentSpans(tmpDir, 10)
	require.NoError(t, err)
	assert.Empty(t, spans)
}

func TestReadRecentSpans_ValidFile(t *testing.T) {
	tmpDir := t.TempDir()
	noctiDir := filepath.Join(tmpDir, ".noctifab")
	require.NoError(t, os.MkdirAll(noctiDir, 0755))

	traceFile := filepath.Join(noctiDir, "traces.jsonl")
	data := `{"name":"RunCommand","duration_ms":120,"status":"OK","attributes":{"command":"go test -v ./..."}}
{"name":"ValidateTask","duration_ms":450,"status":"Error","attributes":{"task.id":"task-1","error":"test failure"}}
`
	require.NoError(t, os.WriteFile(traceFile, []byte(data), 0644))

	spans, err := ReadRecentSpans(tmpDir, 5)
	require.NoError(t, err)
	require.Len(t, spans, 2)

	assert.Equal(t, "RunCommand", spans[0].Name)
	assert.Equal(t, int64(120), spans[0].DurationMS)
	assert.Equal(t, "ValidateTask", spans[1].Name)
	assert.Equal(t, "Error", spans[1].Status)

	formatted := FormatSpansForPrompt(spans)
	assert.Contains(t, formatted, "Recent OpenTelemetry")
	assert.Contains(t, formatted, "RunCommand")
	assert.Contains(t, formatted, "ValidateTask")
	assert.Contains(t, formatted, "go test -v ./...")
}
