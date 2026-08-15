package reporting

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type LLMReportAnalyzer struct {
	client domain.LLMClient
	model  string
}

func NewLLMReportAnalyzer(client domain.LLMClient, model string) *LLMReportAnalyzer {
	return &LLMReportAnalyzer{
		client: client,
		model:  model,
	}
}

type analyzerSubmitArgs struct {
	Summary    string                      `json:"summary"`
	Priorities []domain.AnalysisPriority   `json:"priorities"`
	Hypotheses []domain.AnalysisHypothesis `json:"hypotheses"`
	Proposals  []domain.ReportProposal     `json:"proposals"`
}

func (a *LLMReportAnalyzer) Analyze(ctx context.Context, input domain.ExecutionReportInput) (domain.ExecutionReport, error) {
	if a.client == nil {
		return domain.ExecutionReport{}, fmt.Errorf("llm client is nil")
	}

	payloadBytes, err := json.Marshal(input)
	if err != nil {
		return domain.ExecutionReport{}, fmt.Errorf("failed to marshal report input: %w", err)
	}

	if len(payloadBytes) > 65536 {
		// Truncate payload bytes cleanly
		payloadBytes = payloadBytes[:65536]
	}

	prompt := fmt.Sprintf("Analyze the execution report input snapshot and respond using the submit_report_analysis tool action:\n%s", string(payloadBytes))

	resp, err := a.client.Complete(ctx, prompt)
	if err != nil {
		return domain.ExecutionReport{}, fmt.Errorf("llm analyzer call failed: %w", err)
	}

	for _, action := range resp.Actions {
		if action.Tool == "submit_report_analysis" {
			argsBytes, mErr := json.Marshal(action.Args)
			if mErr != nil {
				continue
			}

			var submitArgs analyzerSubmitArgs
			if uErr := json.Unmarshal(argsBytes, &submitArgs); uErr != nil {
				continue
			}

			// Validate hypotheses confidence
			for i, hyp := range submitArgs.Hypotheses {
				conf := strings.ToLower(hyp.Confidence)
				if conf != "high" && conf != "medium" && conf != "low" {
					submitArgs.Hypotheses[i].Confidence = "medium"
				}
			}

			return domain.ExecutionReport{
				Summary:    submitArgs.Summary,
				Priorities: submitArgs.Priorities,
				Hypotheses: submitArgs.Hypotheses,
				Proposals:  submitArgs.Proposals,
			}, nil
		}
	}

	return domain.ExecutionReport{}, fmt.Errorf("llm response contained no submit_report_analysis action")
}
