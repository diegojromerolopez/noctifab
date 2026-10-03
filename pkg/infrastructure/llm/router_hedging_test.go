package llm

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type delayMockLLM struct {
	delay time.Duration
	resp  *domain.LLMResponse
	err   error
}

func (m *delayMockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	select {
	case <-time.After(m.delay):
		return m.resp, m.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestCompleteWithHedging_PrimaryWins(t *testing.T) {
	t.Parallel()
	primary := RouterCandidate{
		Name:     "primary-fast",
		Provider: "provider1",
		Client: &delayMockLLM{
			delay: 10 * time.Millisecond,
			resp:  &domain.LLMResponse{Reasoning: "from primary"},
		},
	}
	secondary := RouterCandidate{
		Name:     "secondary-slow",
		Provider: "provider2",
		Client: &delayMockLLM{
			delay: 200 * time.Millisecond,
			resp:  &domain.LLMResponse{Reasoning: "from secondary"},
		},
	}

	router := &ResilientLLMRouter{
		hedgeDelay: 50 * time.Millisecond,
		cooldowns:  make(map[string]time.Time),
	}

	resp, err := router.completeWithHedging(context.Background(), "generator", []RouterCandidate{primary, secondary}, "hello")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "from primary", resp.Reasoning)
}

func TestCompleteWithHedging_SecondaryWinsOnStall(t *testing.T) {
	t.Parallel()
	// Primary takes 300ms
	primary := RouterCandidate{
		Name:     "primary-stalled",
		Provider: "provider1",
		Client: &delayMockLLM{
			delay: 300 * time.Millisecond,
			resp:  &domain.LLMResponse{Reasoning: "from primary"},
		},
	}
	// Secondary takes only 10ms once launched
	secondary := RouterCandidate{
		Name:     "secondary-fast",
		Provider: "provider2",
		Client: &delayMockLLM{
			delay: 10 * time.Millisecond,
			resp:  &domain.LLMResponse{Reasoning: "from secondary"},
		},
	}

	router := &ResilientLLMRouter{
		hedgeDelay: 20 * time.Millisecond,
		cooldowns:  make(map[string]time.Time),
	}

	t0 := time.Now()
	resp, err := router.completeWithHedging(context.Background(), "generator", []RouterCandidate{primary, secondary}, "hello")
	elapsed := time.Since(t0)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "from secondary", resp.Reasoning)
	// Must complete well before the 300ms primary delay
	assert.Less(t, elapsed, 150*time.Millisecond)
}

func TestCompleteWithHedging_PrimaryFailsEarly_SecondaryLaunchedImmediately(t *testing.T) {
	t.Parallel()
	primary := RouterCandidate{
		Name:     "primary-fail",
		Provider: "provider1",
		Client: &delayMockLLM{
			delay: 5 * time.Millisecond,
			err:   errors.New("primary network error"),
		},
	}
	secondary := RouterCandidate{
		Name:     "secondary-recovery",
		Provider: "provider2",
		Client: &delayMockLLM{
			delay: 10 * time.Millisecond,
			resp:  &domain.LLMResponse{Reasoning: "recovered by secondary"},
		},
	}

	router := &ResilientLLMRouter{
		hedgeDelay: 100 * time.Millisecond,
		cooldowns:  make(map[string]time.Time),
	}

	t0 := time.Now()
	resp, err := router.completeWithHedging(context.Background(), "generator", []RouterCandidate{primary, secondary}, "hello")
	elapsed := time.Since(t0)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "recovered by secondary", resp.Reasoning)
	// Should not wait for the 100ms timer
	assert.Less(t, elapsed, 50*time.Millisecond)
}

func TestCompleteWithHedging_AdaptiveSpeculativeHedging(t *testing.T) {
	t.Parallel()
	primary := RouterCandidate{
		Name:     "primary-degraded",
		Provider: "provider1",
		Client: &delayMockLLM{
			delay: 200 * time.Millisecond,
			resp:  &domain.LLMResponse{Reasoning: "from primary late"},
		},
	}
	secondary := RouterCandidate{
		Name:     "secondary-fast",
		Provider: "provider2",
		Client: &delayMockLLM{
			delay: 10 * time.Millisecond,
			resp:  &domain.LLMResponse{Reasoning: "from secondary fast"},
		},
	}

	tracker := NewLatencyTracker()
	// Record 2 consecutive timeouts for primary-degraded
	tracker.RecordOutcome(primary.Name, 5*time.Second, context.DeadlineExceeded, 1*time.Second)
	tracker.RecordOutcome(primary.Name, 5*time.Second, context.DeadlineExceeded, 1*time.Second)

	router := &ResilientLLMRouter{
		hedgeDelay:     80 * time.Millisecond, // Standard hedge delay scaled down in test
		latencyTracker: tracker,
		cooldowns:      make(map[string]time.Time),
	}

	t0 := time.Now()
	resp, err := router.completeWithHedging(context.Background(), "generator", []RouterCandidate{primary, secondary}, "prompt")
	elapsed := time.Since(t0)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "from secondary fast", resp.Reasoning)
	// With 2 timeouts, hedge delay drops to 20ms, completing around 30ms (well under 150ms)
	assert.Less(t, elapsed, 100*time.Millisecond)
}

type streamingMockLLM struct {
	chunks   int
	interval time.Duration
	resp     *domain.LLMResponse
	err      error
}

func (m *streamingMockLLM) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	tracker := domain.StreamLivenessTrackerFromContext(ctx)
	for i := 0; i < m.chunks; i++ {
		select {
		case <-time.After(m.interval):
			if tracker != nil {
				tracker.RecordChunk()
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return m.resp, m.err
}

func TestCompleteWithHedging_StreamLivenessPostponesHedge(t *testing.T) {
	t.Parallel()
	primary := RouterCandidate{
		Name:     "primary-streaming",
		Provider: "provider1",
		Client: &streamingMockLLM{
			chunks:   4,
			interval: 15 * time.Millisecond, // Total 60ms, emits chunk every 15ms
			resp:     &domain.LLMResponse{Reasoning: "primary streaming finished"},
		},
	}
	secondary := RouterCandidate{
		Name:     "secondary-fast",
		Provider: "provider2",
		Client: &delayMockLLM{
			delay: 5 * time.Millisecond,
			resp:  &domain.LLMResponse{Reasoning: "from secondary"},
		},
	}

	router := &ResilientLLMRouter{
		hedgeDelay: 20 * time.Millisecond, // Would fire at 20ms if not streaming
		cooldowns:  make(map[string]time.Time),
	}

	resp, err := router.completeWithHedging(context.Background(), "generator", []RouterCandidate{primary, secondary}, "prompt")
	require.NoError(t, err)
	require.NotNil(t, resp)
	// Primary should win because active streaming postponed the hedge
	assert.Equal(t, "primary streaming finished", resp.Reasoning)
}
