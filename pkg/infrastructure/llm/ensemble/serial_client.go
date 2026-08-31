package ensemble

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// SerialClient implements the Sequential Multi-Stage Refinement Pipeline with deterministic Early Exit.
type SerialClient struct {
	stages                 []StageClient
	earlyExitOnPass        bool
	fallbackOnStageFailure bool
	timeout                time.Duration
}

// NewSerialClient creates a new SerialClient.
func NewSerialClient(
	stages []StageClient,
	earlyExitOnPass bool,
	fallbackOnStageFailure bool,
	timeout time.Duration,
) *SerialClient {
	return &SerialClient{
		stages:                 stages,
		earlyExitOnPass:        earlyExitOnPass,
		fallbackOnStageFailure: fallbackOnStageFailure,
		timeout:                timeout,
	}
}

// Complete executes sequential refinement across configured stages.
func (s *SerialClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if len(s.stages) == 0 {
		return nil, fmt.Errorf("serial ensemble has no stages configured")
	}

	callCtx := ctx
	var cancel context.CancelFunc
	if s.timeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}

	var totalUsages []domain.TokenUsage
	var currentResponse *domain.LLMResponse
	var lastErr error

	for i, stage := range s.stages {
		if callCtx.Err() != nil {
			break
		}

		stagePrompt := prompt
		if i > 0 && currentResponse != nil {
			stagePrompt = s.renderStagePrompt(stage.RefinementPrompt, prompt, currentResponse)
		}

		resp, err := stage.Client.Complete(callCtx, stagePrompt)
		if err != nil {
			lastErr = fmt.Errorf("stage %d (%s) failed: %w", i+1, stage.Name, err)
			if !s.fallbackOnStageFailure {
				return nil, lastErr
			}
			// Fallback: continue pipeline using the previous stage's valid response
			continue
		}

		if resp != nil {
			totalUsages = append(totalUsages, resp.Usage)
			currentResponse = resp

			// Check Early Exit on Stage 1 (or any intermediate stage)
			if s.earlyExitOnPass {
				if valid, _ := ValidateCodeResponse(resp); valid {
					// Clean code with zero stubs, valid syntax: early exit!
					resp.Usage = CombineUsage(totalUsages...)
					return resp, nil
				}
			}
		}
	}

	if currentResponse != nil {
		currentResponse.Usage = CombineUsage(totalUsages...)
		return currentResponse, nil
	}

	return nil, fmt.Errorf("serial refinement pipeline failed: %w", lastErr)
}

func (s *SerialClient) renderStagePrompt(customTemplate, originalPrompt string, prevResp *domain.LLMResponse) string {
	prevJSON, _ := json.MarshalIndent(prevResp.Actions, "", "  ")
	prevOutput := string(prevJSON)
	if prevResp.Reasoning != "" {
		prevOutput = "Reasoning:\n" + prevResp.Reasoning + "\n\nActions:\n" + prevOutput
	}

	if strings.TrimSpace(customTemplate) != "" {
		tpl := customTemplate
		tpl = strings.ReplaceAll(tpl, "{{.OriginalPrompt}}", originalPrompt)
		tpl = strings.ReplaceAll(tpl, "{{.PreviousOutput}}", prevOutput)
		return tpl
	}

	// Default refinement prompt
	return fmt.Sprintf(`You are a Principal Software Engineer reviewing and refining a proposed solution.

ORIGINAL GOAL:
%s

PREVIOUS DRAFT:
%s

CRITIQUE & REFINEMENT INSTRUCTIONS:
1. Identify any missing functions, unhandled edge cases, stubs, or syntax flaws in the previous draft.
2. Produce the complete, fully implemented, and refined final solution adhering strictly to the JSON schema.
`, originalPrompt, prevOutput)
}
