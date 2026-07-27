package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const estTokensPerChar = 4

// NamedClient associates an LLM client with a provider and model name for
// cooldown tracking and cost estimation.
type NamedClient struct {
	Name   string
	Model  string
	Client domain.LLMClient
}

// FailoverClient implements domain.LLMClient with fallback providers, cooldown tracking,
// monetary budget enforcement (via BudgetStore), and optional call-count budget.
type FailoverClient struct {
	mu            sync.RWMutex
	backends      []NamedClient
	cooldowns     map[string]time.Time
	duration      time.Duration
	maxCallBudget int
	callCount     int
	budgetStore   domain.BudgetStore
	maxBudgetUSD  float64
}

var _ domain.LLMClient = (*FailoverClient)(nil)

// NewFailoverClient creates a new FailoverClient.
// cooldownDuration sets how long a backend is skipped after a transient error.
// maxCalls limits the total number of Complete calls across all backends (0 = unlimited).
// budgetStore persists monetary usage; when nil or maxBudgetUSD<=0, budget tracking is skipped.
func NewFailoverClient(backends []NamedClient, cooldownDuration time.Duration, maxCalls int, budgetStore domain.BudgetStore, maxBudgetUSD float64) *FailoverClient {
	if cooldownDuration <= 0 {
		cooldownDuration = 5 * time.Minute
	}
	return &FailoverClient{
		backends:      backends,
		cooldowns:     make(map[string]time.Time),
		duration:      cooldownDuration,
		maxCallBudget: maxCalls,
		budgetStore:   budgetStore,
		maxBudgetUSD:  maxBudgetUSD,
	}
}

// Complete iterates through backends in order, skipping those on cooldown.
// Before each call it checks the monetary budget; after a successful call it
// records estimated token cost.
func (f *FailoverClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "Complete",
		trace.WithAttributes(
			attribute.Int("backend_count", len(f.backends)),
			attribute.Int("max_call_budget", f.maxCallBudget),
		))
	defer span.End()

	if f.maxCallBudget > 0 {
		f.mu.Lock()
		if f.callCount >= f.maxCallBudget {
			f.mu.Unlock()
			return nil, fmt.Errorf("%w: reached limit of %d calls", domain.ErrBudgetExhausted, f.maxCallBudget)
		}
		f.callCount++
		f.mu.Unlock()
	}

	today := time.Now().UTC().Format("2006-01-02")

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

		if err := f.checkBudget(ctx, today, backend.Model); err != nil {
			lastErr = err
			continue
		}

		resp, err := backend.Client.Complete(ctx, prompt)
		if err == nil {
			if err := f.recordUsage(ctx, today, backend.Model, prompt, resp); err != nil {
				return nil, err
			}
			return resp, nil
		}

		lastErr = err
		fmt.Printf("⚠️  [LLM Failover Warning] Backend %s (model %s) failed: %v. Switching to next backend candidate...\n", backend.Name, backend.Model, err)

		if isTransientError(err) {
			f.mu.Lock()
			f.cooldowns[backend.Name] = time.Now().Add(f.duration)
			f.mu.Unlock()
		}
	}

	if lastErr != nil {
		fmt.Printf("❌ [LLM Failover Exhausted] All %d backends failed. Last error: %v\n", len(backends), lastErr)
		return nil, fmt.Errorf("all LLM backends failed. Last error: %w", lastErr)
	}
	fmt.Printf("❌ [LLM Failover Exhausted] No LLM backends available\n")
	return nil, fmt.Errorf("no LLM backends available")
}

func (f *FailoverClient) checkBudget(ctx context.Context, date string, model string) error {
	if f.budgetStore == nil || f.maxBudgetUSD <= 0 {
		return nil
	}
	used, err := f.budgetStore.GetDailyUsage(ctx, date, model)
	if err != nil {
		return fmt.Errorf("budget check: %w", err)
	}
	if used >= f.maxBudgetUSD {
		return fmt.Errorf("%w: daily budget $%.2f exhausted for %s", domain.ErrBudgetExhausted, f.maxBudgetUSD, model)
	}
	return nil
}

func (f *FailoverClient) recordUsage(ctx context.Context, date string, model string, prompt string, resp *domain.LLMResponse) error {
	if f.budgetStore == nil || f.maxBudgetUSD <= 0 {
		return nil
	}
	promptTokens := len(prompt) / estTokensPerChar
	completionTokens := estimateCompletionTokens(resp)
	cost := domain.CostForTokens(model, promptTokens, completionTokens)
	if cost <= 0 {
		return nil
	}
	if err := f.budgetStore.IncrementUsage(ctx, date, model, cost); err != nil {
		return fmt.Errorf("failed to record budget usage: %w", err)
	}
	return nil
}

func estimateCompletionTokens(resp *domain.LLMResponse) int {
	n := len(resp.Reasoning) / estTokensPerChar
	for _, a := range resp.Actions {
		n += len(a.Tool) / estTokensPerChar
		if a.Args != nil {
			data, err := json.Marshal(a.Args)
			if err == nil {
				n += len(data) / estTokensPerChar
			}
		}
	}
	if n < 1 {
		n = 1
	}
	return n
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
