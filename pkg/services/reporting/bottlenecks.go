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

	// 1. BN-PHASE-DOMINANT
	if wallMS > 0 {
		for phase, intervals := range snapshot.PhaseIntervals {
			unionMS := TotalIntervalDurationMS(intervals)
			ratio := float64(unionMS) / float64(wallMS)
			if unionMS >= 1000 && ratio >= 0.30 {
				candidates = append(candidates, domain.ReportBottleneck{
					RuleID:      "BN-PHASE-DOMINANT",
					Scope:       "phase:" + phase,
					Measurement: fmt.Sprintf("%d ms (%.1f%% wall time)", unionMS, ratio*100),
					Impact:      fmt.Sprintf("Phase %s dominated process execution time", phase),
				})
			}
		}
	}

	// 2. BN-OP-DOMINANT
	if wallMS > 0 {
		for opClass, sumMS := range snapshot.OperationSumMS {
			ratio := float64(sumMS) / float64(wallMS)
			if sumMS >= 1000 && ratio >= 0.20 {
				candidates = append(candidates, domain.ReportBottleneck{
					RuleID:      "BN-OP-DOMINANT",
					Scope:       "operation:" + opClass,
					Measurement: fmt.Sprintf("%d ms summed (%.1f%% of wall time)", sumMS, ratio*100),
					Impact:      fmt.Sprintf("Operation class %s consumed significant active time", opClass),
				})
			}
		}
	}

	// 3. BN-RETRY
	if snapshot.RetryCount >= 2 {
		candidates = append(candidates, domain.ReportBottleneck{
			RuleID:      "BN-RETRY",
			Scope:       "execution:retries",
			Measurement: fmt.Sprintf("%d retries observed", snapshot.RetryCount),
			Impact:      "Repeated execution retries delayed completion",
		})
	}

	// 4. BN-TIMEOUT
	for _, issue := range snapshot.Issues {
		if issue.Category == "operational" && (issue.Severity == "high" || issue.Severity == "critical") {
			candidates = append(candidates, domain.ReportBottleneck{
				RuleID:      "BN-TIMEOUT",
				Scope:       issue.Scope,
				Measurement: issue.Behavior,
				Impact:      issue.Impact,
			})
		}
	}

	// 5. BN-TOKEN
	if snapshot.MeasuredTokens >= 10000 {
		candidates = append(candidates, domain.ReportBottleneck{
			RuleID:      "BN-TOKEN",
			Scope:       "tokens:usage",
			Measurement: fmt.Sprintf("%d tokens measured", snapshot.MeasuredTokens),
			Impact:      "High total model token usage",
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
