package services

import (
	"context"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
)

type mockLLMIntentClient struct {
	reasoning string
	intent    string
}

func (m *mockLLMIntentClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return &domain.LLMResponse{
		Reasoning: m.reasoning,
		Actions: []domain.LLMAction{
			{
				Tool: "classify",
				Args: map[string]any{"intent": m.intent},
			},
		},
	}, nil
}

func TestSpecIntentDetector_FastPath(t *testing.T) {
	detector := NewSpecIntentDetector(nil)
	ctx := context.Background()

	approvalPhrases := []string{
		"looks good to me",
		"all right, it's enough",
		"the SPEC looks good to me",
		"I like the SPEC.md already, stop",
		"lgtm",
		"done",
		"stop",
		"approved",
		"perfect",
		"finish",
		"exit",
		":q",
		"ready to build",
		"looks great",
		"I'm satisfied",
		"We're done",
	}

	for _, phrase := range approvalPhrases {
		t.Run("approves_"+phrase, func(t *testing.T) {
			isStop, reason := detector.IsTerminationIntent(ctx, phrase)
			assert.True(t, isStop, "expected phrase %q to be recognized as termination intent", phrase)
			assert.NotEmpty(t, reason)
		})
	}

	refinementPhrases := []string{
		"Add support for PostgreSQL instead of SQLite",
		"Make sure the CLI returns exit code 2 on invalid arguments",
		"Please add a section about TLS certificates",
		"The port should be 9000 not 8080",
		"We need to support JSON output format",
	}

	for _, phrase := range refinementPhrases {
		t.Run("refines_"+phrase, func(t *testing.T) {
			isStop, _ := detector.IsTerminationIntent(ctx, phrase)
			assert.False(t, isStop, "expected phrase %q to NOT be recognized as termination intent", phrase)
		})
	}

	// Empty string
	isStop, _ := detector.IsTerminationIntent(ctx, "   ")
	assert.False(t, isStop)
}

func TestSpecIntentDetector_LLMFallback(t *testing.T) {
	ctx := context.Background()

	// 1. LLM returns APPROVE_AND_STOP
	mockClient := &mockLLMIntentClient{
		reasoning: "User expressed complete satisfaction",
		intent:    "APPROVE_AND_STOP",
	}
	detector := NewSpecIntentDetector(mockClient)
	isStop, reason := detector.IsTerminationIntent(ctx, "I believe everything we discussed is now accurately captured in this specification, thank you")
	assert.True(t, isStop)
	assert.Equal(t, "User expressed complete satisfaction", reason)

	// 2. LLM returns REFINE_SPECIFICATION
	mockClient2 := &mockLLMIntentClient{
		reasoning: "User requested changing the caching TTL",
		intent:    "REFINE_SPECIFICATION",
	}
	detector2 := NewSpecIntentDetector(mockClient2)
	isStop2, _ := detector2.IsTerminationIntent(ctx, "Could we also change the default TTL to 60 seconds across all endpoints?")
	assert.False(t, isStop2)
}

func TestSpecIntentDetector_TimeTravel(t *testing.T) {
	detector := NewSpecIntentDetector(nil)

	isTT, op, ver := detector.DetectTimeTravelIntent("undo")
	assert.True(t, isTT)
	assert.Equal(t, "undo", op)
	assert.Equal(t, 0, ver)

	isTT, op, _ = detector.DetectTimeTravelIntent("u")
	assert.True(t, isTT)
	assert.Equal(t, "undo", op)

	isTT, op, _ = detector.DetectTimeTravelIntent("redo")
	assert.True(t, isTT)
	assert.Equal(t, "redo", op)

	isTT, op, _ = detector.DetectTimeTravelIntent("r")
	assert.True(t, isTT)
	assert.Equal(t, "redo", op)

	isTT, op, _ = detector.DetectTimeTravelIntent("history")
	assert.True(t, isTT)
	assert.Equal(t, "history", op)

	isTT, op, _ = detector.DetectTimeTravelIntent("log")
	assert.True(t, isTT)
	assert.Equal(t, "history", op)

	isTT, op, ver = detector.DetectTimeTravelIntent("checkout 2")
	assert.True(t, isTT)
	assert.Equal(t, "checkout", op)
	assert.Equal(t, 2, ver)

	isTT, op, ver = detector.DetectTimeTravelIntent("co v3")
	assert.True(t, isTT)
	assert.Equal(t, "checkout", op)
	assert.Equal(t, 3, ver)

	isTT, _, _ = detector.DetectTimeTravelIntent("Add TLS support")
	assert.False(t, isTT)
}
