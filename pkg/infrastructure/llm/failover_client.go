package llm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// NamedClient associates an LLM client with a provider name for cooldown tracking
type NamedClient struct {
	Name   string
	Client domain.LLMClient
}

// FailoverClient implements domain.LLMClient with fallback providers, cooldown tracking,
// and optional call budget enforcement.
type FailoverClient struct {
	mu          sync.RWMutex
	backends    []NamedClient
	cooldowns   map[string]time.Time
	duration    time.Duration
	maxCallBudget int
	callCount   int
}

var _ domain.LLMClient = (*FailoverClient)(nil)

// NewFailoverClient creates a new FailoverClient.
// cooldownDuration sets how long a backend is skipped after a transient error.
// maxCalls limits the total number of Complete calls across all backends (0 = unlimited).
func NewFailoverClient(backends []NamedClient, cooldownDuration time.Duration, maxCalls int) *FailoverClient {
	if cooldownDuration <= 0 {
		cooldownDuration = 5 * time.Minute
	}
	return &FailoverClient{
		backends:    backends,
		cooldowns:  make(map[string]time.Time),
		duration:   cooldownDuration,
		maxCallBudget: maxCalls,
	}
}

// Complete iterates through backends in order, skipping those on cooldown.
// Returns domain.ErrBudgetExhausted if the call budget has been reached.
func (f *FailoverClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	f.mu.Lock()
	if f.maxCallBudget > 0 && f.callCount >= f.maxCallBudget {
		f.mu.Unlock()
		return nil, fmt.Errorf("%w: reached limit of %d calls", domain.ErrBudgetExhausted, f.maxCallBudget)
	}
	f.callCount++
	f.mu.Unlock()

	var lastErr error

	f.mu.RLock()
	backends := make([]NamedClient, len(f.backends))
	copy(backends, f.backends)
	f.mu.RUnlock()

	for _, backend := range backends {
		f.mu.RLock()
		coolUntil, onCooldown := f.cooldowns[backend.Name]
		f.mu.RUnlock()

		if onCooldown && time.Now().Before(coolUntil) {
			continue
		}

		resp, err := backend.Client.Complete(ctx, prompt)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// If transient API error, place on cooldown
		if isTransientError(err) {
			f.mu.Lock()
			f.cooldowns[backend.Name] = time.Now().Add(f.duration)
			f.mu.Unlock()
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("all LLM backends failed. Last error: %w", lastErr)
	}
	return nil, fmt.Errorf("no LLM backends available")
}

func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "overloaded") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded")
}
