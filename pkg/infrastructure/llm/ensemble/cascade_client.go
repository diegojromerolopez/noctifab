package ensemble

import (
	"context"
	"fmt"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// CascadeClient implements Tiered Fast-Path Escalation (cascade).
type CascadeClient struct {
	tiers   []NamedClient
	timeout time.Duration
}

// NewCascadeClient creates a new CascadeClient.
func NewCascadeClient(tiers []NamedClient, timeout time.Duration) *CascadeClient {
	return &CascadeClient{
		tiers:   tiers,
		timeout: timeout,
	}
}

// Complete executes tiered escalation from fast to frontier models.
func (c *CascadeClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if len(c.tiers) == 0 {
		return nil, fmt.Errorf("cascade ensemble has no tiers configured")
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	var totalUsages []domain.TokenUsage
	var lastErr error
	var lastResp *domain.LLMResponse

	for i, tier := range c.tiers {
		if callCtx.Err() != nil {
			break
		}

		resp, err := tier.Client.Complete(callCtx, prompt)
		if resp != nil {
			totalUsages = append(totalUsages, resp.Usage)
			lastResp = resp
		}
		if err != nil {
			lastErr = fmt.Errorf("tier %d (%s) failed: %w", i+1, tier.Name, err)
			continue
		}

		if resp != nil {
			// Check if response passes validation
			if valid, _ := ValidateCodeResponse(resp); valid {
				resp.Usage = CombineUsage(totalUsages...)
				return resp, nil
			}
			// Response had stubs or syntax issues, escalate to next tier
			lastErr = fmt.Errorf("tier %d (%s) output failed code validation; escalating to next tier", i+1, tier.Name)
		}
	}

	if lastResp != nil {
		lastResp.Usage = CombineUsage(totalUsages...)
		return lastResp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("cascade strategy failed: %w", lastErr)
	}
	return nil, fmt.Errorf("all cascade tiers failed")
}
