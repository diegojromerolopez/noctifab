package services

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func (a *AcceptanceAuditor) formatStoryContracts(state *domain.State) string {
	if state == nil || len(state.StoryContracts) == 0 {
		return "None"
	}
	var sb strings.Builder
	for _, c := range state.StoryContracts {
		fmt.Fprintf(&sb, "Story ID: %s (File: %s)\n", c.StoryID, c.SourcePath)
		for _, pc := range c.PublicContracts {
			fmt.Fprintf(&sb, " - Contract [%s]: interface=%s allowed_executables=%v\n", pc.ID, pc.Interface, pc.AllowedExecutables)
		}
	}
	return sb.String()
}

func (a *AcceptanceAuditor) formatPublicContracts(state *domain.State) string {
	if state == nil || len(state.StoryContracts) == 0 {
		return "None"
	}
	var contracts []string
	for _, c := range state.StoryContracts {
		for _, pc := range c.PublicContracts {
			contracts = append(contracts, fmt.Sprintf("%s (%s)", pc.ID, pc.Interface))
		}
	}
	return strings.Join(contracts, ", ")
}

func (a *AcceptanceAuditor) formatTaskSummaries(state *domain.State) string {
	if state == nil || len(state.Tasks) == 0 {
		return "None"
	}
	var sb strings.Builder
	for _, t := range state.Tasks {
		fmt.Fprintf(&sb, "- [%s] %s: %s (files: %v)\n", t.ID, t.Status, t.Title, t.TargetFiles)
	}
	return sb.String()
}

func (a *AcceptanceAuditor) parseAuditResponse(resp *domain.LLMResponse) *AcceptanceAuditResult {
	if resp == nil {
		return &AcceptanceAuditResult{Passed: false, Summary: "Empty LLM response received"}
	}

	for _, act := range resp.Actions {
		if act.Tool == "submit_acceptance_audit" {
			passed, _ := act.Args["passed"].(bool)
			summary, _ := act.Args["summary"].(string)
			var gaps []string
			if rawGaps, ok := act.Args["gaps"].([]any); ok {
				for _, g := range rawGaps {
					if gs, ok := g.(string); ok && strings.TrimSpace(gs) != "" {
						gaps = append(gaps, strings.TrimSpace(gs))
					}
				}
			}
			var fixes []ProposedFix
			if rawFixes, ok := act.Args["fixes"].([]any); ok {
				for _, f := range rawFixes {
					if fm, ok := f.(map[string]any); ok {
						file, _ := fm["file"].(string)
						desc, _ := fm["description"].(string)
						action, _ := fm["action"].(string)
						if file != "" || desc != "" {
							fixes = append(fixes, ProposedFix{
								File:        file,
								Description: desc,
								Action:      action,
							})
						}
					}
				}
			}
			return &AcceptanceAuditResult{
				Passed:  passed && len(gaps) == 0,
				Summary: summary,
				Gaps:    gaps,
				Fixes:   fixes,
			}
		}
	}

	// Fallback JSON parsing from reasoning or content
	if strings.Contains(resp.Reasoning, "submit_acceptance_audit") || strings.Contains(resp.Reasoning, "\"passed\"") {
		var parsed struct {
			Passed  bool          `json:"passed"`
			Summary string        `json:"summary"`
			Gaps    []string      `json:"gaps"`
			Fixes   []ProposedFix `json:"fixes"`
		}
		start := strings.Index(resp.Reasoning, "{")
		end := strings.LastIndex(resp.Reasoning, "}")
		if start >= 0 && end > start {
			if err := json.Unmarshal([]byte(resp.Reasoning[start:end+1]), &parsed); err == nil {
				return &AcceptanceAuditResult{
					Passed:  parsed.Passed && len(parsed.Gaps) == 0,
					Summary: parsed.Summary,
					Gaps:    parsed.Gaps,
					Fixes:   parsed.Fixes,
				}
			}
		}
	}

	summary := strings.TrimSpace(resp.Reasoning)
	if summary == "" {
		summary = "Acceptance audit evaluated"
	}
	return &AcceptanceAuditResult{
		Passed:  false,
		Summary: "Acceptance audit response could not be parsed: " + summary,
	}
}

// RecordStoryVerified registers a completed user story in the incremental acceptance ledger.
func (a *AcceptanceAuditor) RecordStoryVerified(storyID string, verifiedFeatures []string) {
	if a == nil || storyID == "" {
		return
	}
	a.ledgerMu.Lock()
	defer a.ledgerMu.Unlock()
	if a.verifiedStories == nil {
		a.verifiedStories = make(map[string]bool)
	}
	a.verifiedStories[storyID] = true
}

// IsStoryVerified checks if a story has been incrementally verified.
func (a *AcceptanceAuditor) IsStoryVerified(storyID string) bool {
	if a == nil || storyID == "" {
		return false
	}
	a.ledgerMu.Lock()
	defer a.ledgerMu.Unlock()
	return a.verifiedStories[storyID]
}
