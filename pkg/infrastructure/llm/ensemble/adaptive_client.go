package ensemble

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// AdaptiveClient dynamically routes LLM tasks to the optimal ensemble tier based on task complexity.
type AdaptiveClient struct {
	FastClient     domain.LLMClient
	StandardClient domain.LLMClient
	HeavyClient    domain.LLMClient
	Timeout        time.Duration
}

// NewAdaptiveClient creates a new adaptive ensembling client.
func NewAdaptiveClient(fast, standard, heavy domain.LLMClient, timeout time.Duration) *AdaptiveClient {
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &AdaptiveClient{
		FastClient:     fast,
		StandardClient: standard,
		HeavyClient:    heavy,
		Timeout:        timeout,
	}
}

// Complete dynamically classifies the prompt and dispatches to the optimal tier.
func (a *AdaptiveClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	GlobalTelemetry().RecordInvocation("adaptive")

	ctx, cancel := context.WithTimeout(ctx, a.Timeout)
	defer cancel()

	tierName, targetClient := a.classifyAndSelect(prompt)
	GlobalTelemetry().RecordAdaptivePath(tierName)

	if targetClient == nil {
		targetClient = a.fallbackClient()
		if targetClient == nil {
			return nil, fmt.Errorf("no clients available in adaptive ensemble")
		}
	}

	resp, err := targetClient.Complete(ctx, prompt)
	if err != nil {
		// Fallback to standard client if fast/heavy tier fails
		if fb := a.fallbackClient(); fb != nil && fb != targetClient {
			return fb.Complete(ctx, prompt)
		}
		return nil, fmt.Errorf("adaptive ensemble error (%s tier): %w", tierName, err)
	}

	return resp, nil
}

func (a *AdaptiveClient) classifyAndSelect(prompt string) (string, domain.LLMClient) {
	lower := strings.ToLower(prompt)

	// 1. Heavy Tier: Concurrency, architecture, low-level binary, algorithms, or error remediation
	heavyKeywords := []string{
		"concurren", "asyncio", "goroutine", "mutex", "lock", "atomic",
		"syscall", "assembly", "aarch64", "register", "stack alignment",
		"wire protocol", "resp", "btree", "transaction", "deadlock",
		"segmentation fault", "panic", "fatal", "test failed", "remediation",
		"compile error", "build failed", "rfc",
	}
	for _, kw := range heavyKeywords {
		if strings.Contains(lower, kw) {
			if a.HeavyClient != nil {
				return "heavy", a.HeavyClient
			}
			break
		}
	}

	// 2. Fast Tier: Documentation, comments, typos, minor formatting
	fastKeywords := []string{
		"typo", "comment", "docstring", "readme", "format", "spelling",
		"version bump", "changelog", "rename", "license",
	}
	for _, kw := range fastKeywords {
		if strings.Contains(lower, kw) {
			if a.FastClient != nil {
				return "fast", a.FastClient
			}
			break
		}
	}

	// 3. Standard Tier: General implementation
	if a.StandardClient != nil {
		return "standard", a.StandardClient
	}
	if a.HeavyClient != nil {
		return "heavy", a.HeavyClient
	}
	return "fast", a.FastClient
}

func (a *AdaptiveClient) fallbackClient() domain.LLMClient {
	if a.StandardClient != nil {
		return a.StandardClient
	}
	if a.HeavyClient != nil {
		return a.HeavyClient
	}
	return a.FastClient
}
