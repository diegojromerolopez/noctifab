package ensemble

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ScoredClient implements Deterministic Scored Selection (best_of_n_scored).
type ScoredClient struct {
	models  []NamedClient
	timeout time.Duration
}

// NewScoredClient creates a new ScoredClient.
func NewScoredClient(models []NamedClient, timeout time.Duration) *ScoredClient {
	return &ScoredClient{
		models:  models,
		timeout: timeout,
	}
}

// Complete executes all candidate models in parallel and promotes the highest scoring completion.
func (s *ScoredClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if len(s.models) == 0 {
		return nil, fmt.Errorf("best_of_n_scored ensemble has no models configured")
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if s.timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	results := make([]modelResult, len(s.models))
	var wg sync.WaitGroup

	for i, m := range s.models {
		wg.Add(1)
		go func(idx int, nc NamedClient) {
			defer wg.Done()
			resp, err := nc.Client.Complete(callCtx, prompt)
			results[idx] = modelResult{
				name: nc.Name,
				resp: resp,
				err:  err,
			}
		}(i, m)
	}

	wg.Wait()

	var totalUsages []domain.TokenUsage
	var validResults []modelResult
	var lastErr error

	for _, r := range results {
		if r.resp != nil {
			totalUsages = append(totalUsages, r.resp.Usage)
		}
		if r.err != nil {
			lastErr = r.err
		} else if r.resp != nil && len(r.resp.Actions) > 0 {
			validResults = append(validResults, r)
		}
	}

	if len(validResults) == 0 {
		if lastErr != nil {
			return nil, fmt.Errorf("best_of_n_scored strategy failed: %w", lastErr)
		}
		return nil, fmt.Errorf("no model produced valid actions in best_of_n_scored strategy")
	}

	var bestResp *domain.LLMResponse
	bestScore := -999999

	for _, r := range validResults {
		score := ScoreCodeResponse(r.resp)
		if score > bestScore || bestResp == nil {
			bestScore = score
			bestResp = r.resp
		}
	}

	bestResp.Usage = CombineUsage(totalUsages...)
	return bestResp, nil
}
