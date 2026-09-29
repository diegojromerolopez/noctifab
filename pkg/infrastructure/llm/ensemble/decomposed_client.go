package ensemble

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// DecomposedClient implements Divide-and-Conquer Multi-File Generation (decomposed).
type DecomposedClient struct {
	targets []TargetClient
	timeout time.Duration
}

// NewDecomposedClient creates a new DecomposedClient.
func NewDecomposedClient(targets []TargetClient, timeout time.Duration) *DecomposedClient {
	return &DecomposedClient{
		targets: targets,
		timeout: timeout,
	}
}

// Complete executes parallel specialist generation and merges resulting tool actions.
func (d *DecomposedClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if len(d.targets) == 0 {
		return nil, fmt.Errorf("decomposed ensemble has no targets configured")
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if d.timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, d.timeout)
		defer cancel()
	}

	type targetResult struct {
		name string
		resp *domain.LLMResponse
		err  error
	}

	results := make([]targetResult, len(d.targets))
	var wg sync.WaitGroup

	for i, t := range d.targets {
		wg.Add(1)
		go func(idx int, tc TargetClient) {
			defer wg.Done()
			targetPrompt := prompt
			if strings.TrimSpace(tc.RolePrompt) != "" {
				targetPrompt = fmt.Sprintf("%s\n\nSPECIALIST DIRECTIVE (%s):\n%s", prompt, tc.Name, tc.RolePrompt)
			}
			resp, err := tc.Client.Complete(callCtx, targetPrompt)
			results[idx] = targetResult{
				name: tc.Name,
				resp: resp,
				err:  err,
			}
		}(i, t)
	}

	wg.Wait()

	var totalUsages []domain.TokenUsage
	var actionsLists [][]domain.LLMAction
	var reasonings []string
	var errorsList []string

	for _, r := range results {
		if r.resp != nil {
			totalUsages = append(totalUsages, r.resp.Usage)
			if len(r.resp.Actions) > 0 {
				actionsLists = append(actionsLists, r.resp.Actions)
			}
			if r.resp.Reasoning != "" {
				reasonings = append(reasonings, fmt.Sprintf("[%s]: %s", r.name, r.resp.Reasoning))
			}
		}
		if r.err != nil {
			errorsList = append(errorsList, fmt.Sprintf("%s: %v", r.name, r.err))
		}
	}

	mergedActions := MergeActions(actionsLists...)
	if len(mergedActions) == 0 {
		return nil, fmt.Errorf("decomposed generation produced no valid actions (errors: %s)", strings.Join(errorsList, "; "))
	}

	return &domain.LLMResponse{
		Reasoning: strings.Join(reasonings, "\n\n"),
		Actions:   mergedActions,
		Usage:     CombineUsage(totalUsages...),
	}, nil
}
