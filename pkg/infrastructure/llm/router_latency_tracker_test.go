package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatencyTracker_DemotionOnTimeout(t *testing.T) {
	lt := NewLatencyTracker()

	candidates := []RouterCandidate{
		{Name: "slow-primary", Provider: "anthropic", Model: "claude-opus-5"},
		{Name: "fast-secondary", Provider: "openai", Model: "gpt-5.6-luna"},
		{Name: "fast-tertiary", Provider: "gemini", Model: "gemini-3.8-flash"},
	}

	// 1. Initial state: no demotion
	res := lt.ApplyDynamicDemotion(candidates)
	require.Len(t, res, 3)
	assert.Equal(t, "slow-primary", res[0].Name)
	assert.Equal(t, "fast-secondary", res[1].Name)
	assert.Equal(t, "fast-tertiary", res[2].Name)

	// 2. Single timeout event: slow-primary demoted by 1 position (to 2nd place)
	lt.RecordOutcome("slow-primary", 65*time.Second, context.DeadlineExceeded, 60*time.Second)
	assert.Equal(t, 1, lt.GetPenaltyScore("slow-primary"))

	res1 := lt.ApplyDynamicDemotion(candidates)
	assert.Equal(t, "fast-secondary", res1[0].Name)
	assert.Equal(t, "slow-primary", res1[1].Name)
	assert.Equal(t, "fast-tertiary", res1[2].Name)

	// 3. Second timeout event: slow-primary demoted by 2 positions (to 3rd place)
	lt.RecordOutcome("slow-primary", 70*time.Second, errors.New("context canceled"), 60*time.Second)
	assert.Equal(t, 2, lt.GetPenaltyScore("slow-primary"))

	res2 := lt.ApplyDynamicDemotion(candidates)
	assert.Equal(t, "fast-secondary", res2[0].Name)
	assert.Equal(t, "fast-tertiary", res2[1].Name)
	assert.Equal(t, "slow-primary", res2[2].Name)

	// 4. Fast success decays penalty
	lt.RecordOutcome("slow-primary", 10*time.Second, nil, 60*time.Second)
	assert.Equal(t, 1, lt.GetPenaltyScore("slow-primary"))

	lt.RecordOutcome("slow-primary", 12*time.Second, nil, 60*time.Second)
	assert.Equal(t, 0, lt.GetPenaltyScore("slow-primary"))

	res3 := lt.ApplyDynamicDemotion(candidates)
	assert.Equal(t, "slow-primary", res3[0].Name)
	assert.Equal(t, "fast-secondary", res3[1].Name)
	assert.Equal(t, "fast-tertiary", res3[2].Name)
}

func TestLatencyTracker_HighLatencyPenalty(t *testing.T) {
	lt := NewLatencyTracker()

	candidates := []RouterCandidate{
		{Name: "c1", Provider: "anthropic"},
		{Name: "c2", Provider: "openai"},
	}

	// Successful call but took > 60s
	lt.RecordOutcome("c1", 75*time.Second, nil, 180*time.Second)
	assert.Equal(t, 1, lt.GetPenaltyScore("c1"))

	res := lt.ApplyDynamicDemotion(candidates)
	assert.Equal(t, "c2", res[0].Name)
	assert.Equal(t, "c1", res[1].Name)
}
