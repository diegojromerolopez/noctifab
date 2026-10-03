package cli

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
)

type alternatingSandbox struct {
	mu    sync.Mutex
	count int
}

func (s *alternatingSandbox) RunCommand(ctx context.Context, dir string, cmd string, pkg string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.count++
	if s.count%2 == 1 {
		return "FAIL: test_collections (test_store.StoreTests)\nAttributeError: 'Store' object has no attribute 'data'", errors.New("exit status 1")
	}
	return "FAIL: test_types (unit.test_store.StoreTests)\nAttributeError: 'Store' object has no attribute 'type'", errors.New("exit status 1")
}

func (s *alternatingSandbox) RunE2E(ctx context.Context, dir string, cmd string) (string, error) {
	return "", nil
}

func (s *alternatingSandbox) IsContainerized() bool {
	return false
}

func (s *alternatingSandbox) Cleanup(ctx context.Context) error {
	return nil
}

func TestSovereignRescue_OscillationCircuitBreaker_Tripping(t *testing.T) {
	t.Run("when failures oscillate between two states across turns, circuit breaker halts loop before maxTurns", func(t *testing.T) {
		tempDir := t.TempDir()
		mockRepo := &mockDAGStateRepo{
			state: &domain.State{ProjectPath: tempDir},
		}

		mockLLM := &mockRescueLLM{
			responses: []*domain.LLMResponse{
				{Actions: []domain.LLMAction{{Tool: "noop"}}},
			},
		}
		reg := services.NewToolRegistry()
		reg.Register(&services.NoopTool{})

		sandbox := &alternatingSandbox{}
		validator := services.NewTestValidator(sandbox, false, mockLLM, reg.Tools())

		opts := SovereignRescueOptions{
			TargetDir:    tempDir,
			Cfg:          config.DefaultConfig(),
			Repo:         mockRepo,
			LLMClient:    mockLLM,
			ToolRegistry: reg,
			Validator:    validator,
			MaxTurns:     15,
		}

		err := DispatchSovereignRescue(context.Background(), opts)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "oscillation circuit breaker")
		// Must have tripped before exhausting all 15 turns!
		assert.Less(t, mockLLM.calls, 15, "circuit breaker must halt before max turns")
		assert.GreaterOrEqual(t, mockLLM.calls, 3, "circuit breaker needs at least 3-4 turns to confirm cycle and trip")
	})
}
