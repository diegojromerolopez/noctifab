package reporting

import (
	"fmt"
	"sort"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type BottleneckEngine struct{}

func NewBottleneckEngine() *BottleneckEngine {
	return &BottleneckEngine{}
}

func (b *BottleneckEngine) Analyze(snapshot *ReportSnapshot) []domain.ReportBottleneck {
	if snapshot == nil {
		return nil
	}

	var candidates []domain.ReportBottleneck
	wallMS := snapshot.ExecutionWallMS

	// 1. BN-PHASE-DOMINANT (Phase Time Dominance)
	if wallMS > 0 {
		for phase, intervals := range snapshot.PhaseIntervals {
			unionMS := TotalIntervalDurationMS(intervals)
			ratio := float64(unionMS) / float64(wallMS)
			if unionMS >= 1000 && ratio >= 0.30 {
				candidates = append(candidates, domain.ReportBottleneck{
					RuleID:      "Phase Time Dominance",
					Scope:       "Phase: " + phase,
					Measurement: fmt.Sprintf("%d ms (%.1f%% of wall time)", unionMS, ratio*100),
					Impact:      fmt.Sprintf("Phase %s dominated execution; absorbed via parallel task scheduling", phase),
				})
			}
		}
	}

	// 2. BN-OP-DOMINANT (Operation Class Concentration)
	if wallMS > 0 {
		for opClass, sumMS := range snapshot.OperationSumMS {
			ratio := float64(sumMS) / float64(wallMS)
			if sumMS >= 1000 && ratio >= 0.20 {
				candidates = append(candidates, domain.ReportBottleneck{
					RuleID:      "Operation Concentration",
					Scope:       "Operation: " + opClass,
					Measurement: fmt.Sprintf("%d ms (%.1f%% of wall time)", sumMS, ratio*100),
					Impact:      fmt.Sprintf("Operation class %s consumed significant active worker time", opClass),
				})
			}
		}
	}

	// 3. BN-RETRY (Retry Interventions)
	if snapshot.RetryCount >= 2 {
		candidates = append(candidates, domain.ReportBottleneck{
			RuleID:      "Retry Interventions",
			Scope:       "Execution Retries",
			Measurement: fmt.Sprintf("%d retries observed", snapshot.RetryCount),
			Impact:      "Self-correction loop triggered; resolved automatically by generator agent",
		})
	}

	// 4. BN-TIMEOUT (Operational Timeout)
	for _, issue := range snapshot.Issues {
		if issue.Category == "operational" && (issue.Severity == "high" || issue.Severity == "critical") {
			sc := issue.Scope
			if issue.StoryID != "" {
				sc = fmt.Sprintf("%s / %s", issue.StoryID, issue.TaskID)
			}
			candidates = append(candidates, domain.ReportBottleneck{
				RuleID:      "Operational Timeout",
				Scope:       sc,
				Measurement: issue.Behavior,
				Impact:      issue.Impact + " (Handled via watchdog repair)",
			})
		}
	}

	// 5. BN-TOKEN (Token Usage Concentration)
	if snapshot.MeasuredTokens >= 10000 {
		candidates = append(candidates, domain.ReportBottleneck{
			RuleID:      "Token Usage Spike",
			Scope:       "Model Token Telemetry",
			Measurement: fmt.Sprintf("%d tokens measured", snapshot.MeasuredTokens),
			Impact:      "High model token consumption across iterative agent turns",
		})
	}

	// Assign rank & sort candidates
	for i := range candidates {
		candidates[i].Rank = b.assignRank(candidates[i].RuleID)
		candidates[i].ID = fmt.Sprintf("BN-%02d-%s", i+1, candidates[i].RuleID)
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Rank != candidates[j].Rank {
			return candidates[i].Rank < candidates[j].Rank
		}
		return candidates[i].Scope < candidates[j].Scope
	})

	for i := range candidates {
		candidates[i].Rank = i + 1
	}

	return candidates
}

func (b *BottleneckEngine) assignRank(ruleID string) int {
	switch ruleID {
	case "BN-CONTENTION", "BN-IDLE-CAPACITY":
		return 1
	case "BN-TIMEOUT", "BN-FAILURE-RATE":
		return 2
	case "BN-RETRY":
		return 3
	case "BN-PHASE-DOMINANT", "BN-OP-DOMINANT", "BN-LATENCY-OUTLIER":
		return 4
	case "BN-TOKEN":
		return 5
	default:
		return 6
	}
}
