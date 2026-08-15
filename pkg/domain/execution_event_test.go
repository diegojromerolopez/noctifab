package domain_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestExecutionEventJSON(t *testing.T) {
	at := time.Date(2026, 8, 11, 22, 0, 0, 0, time.UTC)
	dur := int64(1500)
	exitCode := 0

	event := domain.ExecutionEvent{
		ID:             "event-001",
		RunID:          "run-123",
		Kind:           domain.EventTaskAttemptFinished,
		At:             at,
		DurationMillis: &dur,
		Outcome:        domain.OutcomeSuccess,
		ExitCode:       &exitCode,
		UsageKind:      "exact",
	}

	bytes, err := json.Marshal(event)
	require.NoError(t, err)

	var unmarshaled domain.ExecutionEvent
	err = json.Unmarshal(bytes, &unmarshaled)
	require.NoError(t, err)

	assert.Equal(t, event.ID, unmarshaled.ID)
	require.NotNil(t, unmarshaled.DurationMillis)
	assert.Equal(t, int64(1500), *unmarshaled.DurationMillis)
	require.NotNil(t, unmarshaled.ExitCode)
	assert.Equal(t, 0, *unmarshaled.ExitCode)
}
