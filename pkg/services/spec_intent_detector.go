package services

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// IntentType represents the classified human intention in the spec loop.
type IntentType string

const (
	IntentApproveAndStop    IntentType = "APPROVE_AND_STOP"
	IntentRefineSpec        IntentType = "REFINE_SPECIFICATION"
	IntentTimeTravel        IntentType = "TIME_TRAVEL"
	IntentClarificationHelp IntentType = "CLARIFY_QUESTION"
)

var defaultApprovalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*(looks\s+good(\s+to\s+me)?|lgtm|approved?|perfect|done|stop|enough|it['’]?s\s+enough)\s*[.!]?\s*$`),
	regexp.MustCompile(`(?i)^\s*(all\s+right[,\s]+it['’]?s\s+enough|i\s+like\s+(it|the\s+spec(\.md)?)\s+(already[,\s]+)?stop)\s*[.!]?\s*$`),
	regexp.MustCompile(`(?i)^\s*(save(\s+and\s+exit)?|finish|quit|exit|:q|q)\s*$`),
	regexp.MustCompile(`(?i)^\s*(ready(\s+to\s+build|\s+for\s+roadmap)?|proceed|good\s+to\s+go)\s*[.!]?\s*$`),
	regexp.MustCompile(`(?i)^\s*(that['’]?s\s+all|we['’]?re\s+done|looks\s+great|looks\s+fine|i['’]?m\s+satisfied)\s*[.!]?\s*$`),
}

var checkoutPattern = regexp.MustCompile(`(?i)^\s*(checkout|co)\s+v?(\d+)\s*$`)

// SpecIntentDetector evaluates whether user input in the interactive spec loop indicates termination, time-travel, or refinement.
type SpecIntentDetector struct {
	llmClient domain.LLMClient
	patterns  []*regexp.Regexp
}

// NewSpecIntentDetector creates an intent detector. llmClient may be nil for fast-path-only mode.
func NewSpecIntentDetector(llmClient domain.LLMClient) *SpecIntentDetector {
	return &SpecIntentDetector{
		llmClient: llmClient,
		patterns:  defaultApprovalPatterns,
	}
}

// DetectTimeTravelIntent evaluates whether input is a time-travel command (undo, redo, history, checkout).
func (d *SpecIntentDetector) DetectTimeTravelIntent(input string) (isTimeTravel bool, op string, version int) {
	trimmed := strings.ToLower(strings.TrimSpace(input))
	if trimmed == "undo" || trimmed == "u" {
		return true, "undo", 0
	}
	if trimmed == "redo" || trimmed == "r" {
		return true, "redo", 0
	}
	if trimmed == "history" || trimmed == "log" || trimmed == "hist" {
		return true, "history", 0
	}

	if match := checkoutPattern.FindStringSubmatch(trimmed); len(match) > 2 {
		var ver int
		if _, err := fmt.Sscanf(match[2], "%d", &ver); err == nil && ver > 0 {
			return true, "checkout", ver
		}
	}
	return false, "", 0
}

// IsTerminationIntent evaluates whether the given input means the user wants to approve and stop.
func (d *SpecIntentDetector) IsTerminationIntent(ctx context.Context, input string) (bool, string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return false, ""
	}

	// 1. Fast-path regex matching (instantaneous, offline)
	for _, pattern := range d.patterns {
		if pattern.MatchString(trimmed) {
			return true, fmt.Sprintf("Matched approval phrase %q", trimmed)
		}
	}

	// Also check if text contains clear stop/approval clauses
	lower := strings.ToLower(trimmed)
	if strings.Contains(lower, "looks good to me") ||
		strings.Contains(lower, "all right, it's enough") ||
		strings.Contains(lower, "i like the spec") && strings.Contains(lower, "stop") {
		return true, "Matched multi-clause approval phrase"
	}

	// 2. Fallback to LLM classifier if available and input is conversational (> 15 chars)
	if d.llmClient != nil && len(trimmed) > 15 {
		intent, reason, err := d.classifyWithLLM(ctx, trimmed)
		if err == nil && intent == IntentApproveAndStop {
			return true, reason
		}
	}

	return false, ""
}

type llmIntentResponse struct {
	Intent    string  `json:"intent"`
	Reasoning string  `json:"reasoning"`
	Score     float64 `json:"confidence,omitempty"`
}

func (d *SpecIntentDetector) classifyWithLLM(ctx context.Context, input string) (IntentType, string, error) {
	prompt := fmt.Sprintf(`You are an intent classification classifier in an interactive software specification CLI.
The user is reviewing a generated SPEC.md document.
Classify whether the user input indicates they are SATISFIED/APPROVING the specification and want to stop the review loop (APPROVE_AND_STOP), or if they are requesting modifications, additions, fixes, or changes to the specification (REFINE_SPECIFICATION).

User Input: %q

Return your decision in the JSON envelope with Reasoning explaining why and an action tool 'classify' with args: {"intent": "APPROVE_AND_STOP" | "REFINE_SPECIFICATION"}.`, input)

	resp, err := d.llmClient.Complete(ctx, prompt)
	if err != nil {
		return IntentRefineSpec, "", err
	}

	for _, act := range resp.Actions {
		if intentVal, ok := act.Args["intent"].(string); ok {
			if strings.EqualFold(intentVal, string(IntentApproveAndStop)) {
				return IntentApproveAndStop, resp.Reasoning, nil
			}
			return IntentRefineSpec, resp.Reasoning, nil
		}
	}

	if strings.Contains(strings.ToUpper(resp.Reasoning), "APPROVE_AND_STOP") {
		return IntentApproveAndStop, resp.Reasoning, nil
	}

	var parsed llmIntentResponse
	if err := json.Unmarshal([]byte(resp.Reasoning), &parsed); err == nil && parsed.Intent != "" {
		if strings.EqualFold(parsed.Intent, string(IntentApproveAndStop)) {
			return IntentApproveAndStop, parsed.Reasoning, nil
		}
	}

	return IntentRefineSpec, resp.Reasoning, nil
}
