package ensemble

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ConsensusClient implements Dual-Perspective Consensus Voting (consensus).
type ConsensusClient struct {
	voters     []NamedClient
	tieBreaker domain.LLMClient
	timeout    time.Duration
}

// NewConsensusClient creates a new ConsensusClient.
func NewConsensusClient(
	voters []NamedClient,
	tieBreaker domain.LLMClient,
	timeout time.Duration,
) *ConsensusClient {
	return &ConsensusClient{
		voters:     voters,
		tieBreaker: tieBreaker,
		timeout:    timeout,
	}
}

// Complete executes parallel voting and resolves divergence via tie-breaker.
func (c *ConsensusClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	GlobalTelemetry().RecordInvocation("consensus")
	if len(c.voters) == 0 {
		if c.tieBreaker != nil {
			return c.tieBreaker.Complete(ctx, prompt)
		}
		return nil, fmt.Errorf("consensus ensemble has no voters or tie-breaker configured")
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if c.timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}

	results := make([]modelResult, len(c.voters))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, v := range c.voters {
		wg.Add(1)
		go func(idx int, nc NamedClient) {
			defer wg.Done()
			resp, err := nc.Client.Complete(callCtx, prompt)
			mu.Lock()
			results[idx] = modelResult{
				name: nc.Name,
				resp: resp,
				err:  err,
			}
			mu.Unlock()
		}(i, v)
	}

	wg.Wait()

	var validResults []modelResult
	var totalUsages []domain.TokenUsage

	for _, r := range results {
		if r.resp != nil {
			totalUsages = append(totalUsages, r.resp.Usage)
		}
		if r.err == nil && r.resp != nil {
			validResults = append(validResults, r)
		}
	}

	if len(validResults) == 0 {
		if c.tieBreaker != nil {
			return c.tieBreaker.Complete(ctx, prompt)
		}
		return nil, fmt.Errorf("all consensus voters failed")
	}

	if len(validResults) == 1 || c.isUnanimous(validResults) {
		GlobalTelemetry().RecordConsensus(true)
		best := validResults[0].resp
		best.Usage = CombineUsage(totalUsages...)
		return best, nil
	}

	GlobalTelemetry().RecordConsensus(false)

	// Disagreement: Invoke Tie-Breaker
	if c.tieBreaker == nil {
		best := validResults[0].resp
		best.Usage = CombineUsage(totalUsages...)
		return best, nil
	}

	tiePrompt := c.buildTieBreakerPrompt(prompt, validResults)
	tieResp, tieErr := c.tieBreaker.Complete(ctx, tiePrompt)
	if tieErr != nil || tieResp == nil {
		best := validResults[0].resp
		best.Usage = CombineUsage(totalUsages...)
		return best, nil
	}

	totalUsages = append(totalUsages, tieResp.Usage)
	tieResp.Usage = CombineUsage(totalUsages...)
	return tieResp, nil
}

func (c *ConsensusClient) isUnanimous(results []modelResult) bool {
	if len(results) <= 1 {
		return true
	}

	firstPassed, hasPassed := c.extractPassedFlag(results[0].resp)
	if !hasPassed {
		// If not a pass/fail audit, check if action counts and tools match
		return len(results[0].resp.Actions) == len(results[1].resp.Actions)
	}

	for i := 1; i < len(results); i++ {
		p, ok := c.extractPassedFlag(results[i].resp)
		if !ok || p != firstPassed {
			return false
		}
	}
	return true
}

func (c *ConsensusClient) extractPassedFlag(resp *domain.LLMResponse) (bool, bool) {
	if resp == nil {
		return false, false
	}
	for _, act := range resp.Actions {
		if act.Tool == "submit_story_qa_audit" || act.Tool == "submit_acceptance_audit" || act.Tool == "submit_audit" {
			if passed, ok := act.Args["passed"].(bool); ok {
				return passed, true
			}
		}
	}
	return false, false
}

func (c *ConsensusClient) buildTieBreakerPrompt(originalPrompt string, results []modelResult) string {
	var sb strings.Builder
	sb.WriteString("You are the Supreme Quality Auditor resolving a split verdict between two independent peer reviewers.\n\n")
	sb.WriteString("AUDIT PROMPT / CRITERIA:\n")
	sb.WriteString(originalPrompt)
	sb.WriteString("\n\n---\nREVIEWER ASSESSMENTS:\n")

	for i, r := range results {
		fmt.Fprintf(&sb, "\n### Reviewer %d (%s):\n", i+1, r.name)
		if r.resp != nil {
			if r.resp.Reasoning != "" {
				fmt.Fprintf(&sb, "**Reasoning:** %s\n", r.resp.Reasoning)
			}
			actionsJSON, _ := json.MarshalIndent(r.resp.Actions, "", "  ")
			fmt.Fprintf(&sb, "```json\n%s\n```\n", string(actionsJSON))
		}
	}

	sb.WriteString("\nTIE-BREAKER MANDATE:\n")
	sb.WriteString("1. Carefully evaluate both viewpoints against the target criteria.\n")
	sb.WriteString("2. Render the authoritative, final determination adhering to the required JSON action schema.\n")

	return sb.String()
}
