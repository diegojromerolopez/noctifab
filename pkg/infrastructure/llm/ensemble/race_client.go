package ensemble

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// RaceClient implements the Speculative First-Valid Race strategy.
type RaceClient struct {
	models  []NamedClient
	timeout time.Duration
}

// NewRaceClient creates a new RaceClient.
func NewRaceClient(models []NamedClient, timeout time.Duration) *RaceClient {
	return &RaceClient{
		models:  models,
		timeout: timeout,
	}
}

// Complete executes speculative racing across models, returning the first valid response.
func (r *RaceClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if len(r.models) == 0 {
		return nil, fmt.Errorf("race ensemble has no models configured")
	}

	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	if r.timeout > 0 {
		var timeoutCancel context.CancelFunc
		callCtx, timeoutCancel = context.WithTimeout(callCtx, r.timeout)
		defer timeoutCancel()
	}

	winCh := make(chan modelResult, 1)
	var wg sync.WaitGroup
	var once sync.Once
	var mu sync.Mutex
	var totalUsages []domain.TokenUsage
	var lastErr error

	for _, m := range r.models {
		wg.Add(1)
		go func(nc NamedClient) {
			defer wg.Done()
			resp, err := nc.Client.Complete(callCtx, prompt)

			mu.Lock()
			if resp != nil {
				totalUsages = append(totalUsages, resp.Usage)
			}
			if err != nil {
				lastErr = err
			}
			mu.Unlock()

			if err == nil && resp != nil {
				if valid, _ := ValidateCodeResponse(resp); valid {
					once.Do(func() {
						winCh <- modelResult{name: nc.Name, resp: resp}
						cancel() // Abort remaining slower models
					})
				}
			}
		}(m)
	}

	// In case none passed validation, close when all finish
	go func() {
		wg.Wait()
		close(winCh)
	}()

	winner, ok := <-winCh
	if ok && winner.resp != nil {
		winner.resp.Usage = CombineUsage(totalUsages...)
		return winner.resp, nil
	}

	if lastErr != nil {
		return nil, fmt.Errorf("race strategy failed: %w", lastErr)
	}
	return nil, fmt.Errorf("no model produced a valid response in race strategy")
}
