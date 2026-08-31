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

// ParallelClient implements the Parallel & Posterior Mix ensembling topology with speculative quorum.
type ParallelClient struct {
	models           []NamedClient
	synthesizer      domain.LLMClient
	minModels        int
	softTimeout      time.Duration
	timeout          time.Duration
	fallbackToSingle bool
}

// NewParallelClient creates a new ParallelClient.
func NewParallelClient(
	models []NamedClient,
	synthesizer domain.LLMClient,
	minModels int,
	softTimeout time.Duration,
	timeout time.Duration,
	fallbackToSingle bool,
) *ParallelClient {
	if minModels <= 0 {
		minModels = 1
	}
	if minModels > len(models) {
		minModels = len(models)
	}
	return &ParallelClient{
		models:           models,
		synthesizer:      synthesizer,
		minModels:        minModels,
		softTimeout:      softTimeout,
		timeout:          timeout,
		fallbackToSingle: fallbackToSingle,
	}
}

type modelResult struct {
	name  string
	resp  *domain.LLMResponse
	err   error
	durMS int64
}

// Complete executes fan-out to models and synthesizes the winning proposals.
func (p *ParallelClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if len(p.models) == 0 {
		if p.synthesizer != nil {
			return p.synthesizer.Complete(ctx, prompt)
		}
		return nil, fmt.Errorf("parallel ensemble has no models or synthesizer configured")
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if p.timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	resultsCh := make(chan modelResult, len(p.models))
	var wg sync.WaitGroup

	for _, m := range p.models {
		wg.Add(1)
		go func(nc NamedClient) {
			defer wg.Done()
			start := time.Now()
			resp, err := nc.Client.Complete(callCtx, prompt)
			dur := time.Since(start).Milliseconds()
			resultsCh <- modelResult{
				name:  nc.Name,
				resp:  resp,
				err:   err,
				durMS: dur,
			}
		}(m)
	}

	// Close channel once all finish in background
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	var successfulResults []modelResult
	var totalUsages []domain.TokenUsage
	softTimer := time.NewTimer(p.softTimeout)
	if p.softTimeout <= 0 {
		softTimer.Stop()
	} else {
		defer softTimer.Stop()
	}

collectLoop:
	for {
		select {
		case res, ok := <-resultsCh:
			if !ok {
				break collectLoop
			}
			if res.resp != nil {
				totalUsages = append(totalUsages, res.resp.Usage)
			}
			if res.err == nil && res.resp != nil {
				successfulResults = append(successfulResults, res)
				// Check speculative quorum
				if len(successfulResults) >= p.minModels {
					break collectLoop
				}
			}
		case <-softTimer.C:
			// Soft timeout fired: if we have at least 1 successful model, proceed to synthesis
			if len(successfulResults) >= 1 {
				break collectLoop
			}
		case <-callCtx.Done():
			break collectLoop
		}
	}

	// If no models succeeded
	if len(successfulResults) == 0 {
		if p.fallbackToSingle && p.synthesizer != nil {
			return p.synthesizer.Complete(ctx, prompt)
		}
		return nil, fmt.Errorf("all parallel ensemble models failed and fallback_to_single is false")
	}

	// If we have only 1 model or no synthesizer configured, return the best proposal directly
	if len(successfulResults) == 1 || p.synthesizer == nil {
		best := p.promoteBestResult(successfulResults)
		best.Usage = CombineUsage(totalUsages...)
		return best, nil
	}

	// Synthesize multiple proposals
	synthPrompt := p.buildSynthesizerPrompt(prompt, successfulResults)
	synthResp, synthErr := p.synthesizer.Complete(ctx, synthPrompt)
	if synthErr != nil || synthResp == nil {
		// Best-of-N fallback: promote highest-scoring model proposal directly
		best := p.promoteBestResult(successfulResults)
		best.Usage = CombineUsage(totalUsages...)
		return best, nil
	}

	totalUsages = append(totalUsages, synthResp.Usage)
	synthResp.Usage = CombineUsage(totalUsages...)
	return synthResp, nil
}

func (p *ParallelClient) promoteBestResult(results []modelResult) *domain.LLMResponse {
	var bestResp *domain.LLMResponse
	bestScore := -999999

	for _, r := range results {
		if r.resp == nil {
			continue
		}
		score := ScoreCodeResponse(r.resp)
		if score > bestScore || bestResp == nil {
			bestScore = score
			bestResp = r.resp
		}
	}
	if bestResp == nil && len(results) > 0 {
		return results[0].resp
	}
	return bestResp
}

func (p *ParallelClient) buildSynthesizerPrompt(originalPrompt string, results []modelResult) string {
	var sb strings.Builder
	sb.WriteString("You are the Lead Architect synthesizing solutions from multiple expert AI models.\n\n")
	sb.WriteString("ORIGINAL GOAL / PROMPT:\n")
	sb.WriteString(originalPrompt)
	sb.WriteString("\n\n---\nPROPOSALS FROM SPECIALIST MODELS:\n")

	for i, r := range results {
		fmt.Fprintf(&sb, "\n### Model Proposal %d (%s):\n", i+1, r.name)
		if r.resp != nil {
			if r.resp.Reasoning != "" {
				fmt.Fprintf(&sb, "**Reasoning:** %s\n", r.resp.Reasoning)
			}
			actionsJSON, _ := json.MarshalIndent(r.resp.Actions, "", "  ")
			fmt.Fprintf(&sb, "```json\n%s\n```\n", string(actionsJSON))
		}
	}

	sb.WriteString("\nSYNTHESIS MANDATE:\n")
	sb.WriteString("1. Select the cleanest, most complete architectural approach with zero stubs.\n")
	sb.WriteString("2. Ensure all required files, interfaces, CLI flags, and test assertions are preserved.\n")
	sb.WriteString("3. Respond with the final consolidated JSON actions adhering to the schema.\n")

	return sb.String()
}
