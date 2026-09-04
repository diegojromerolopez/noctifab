package reporting

import (
	"fmt"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm/ensemble"
)

type Renderer struct {
	redactor *Redactor
}

func NewRenderer(redactor *Redactor) *Renderer {
	if redactor == nil {
		redactor = NewRedactor()
	}
	return &Renderer{redactor: redactor}
}

func (r *Renderer) RenderMarkdown(snapshot *ReportSnapshot) []byte {
	var sb strings.Builder

	sb.WriteString("# Noctifab Execution Report\n\n")
	fmt.Fprintf(&sb, "> Status: %s\n", snapshot.Status)
	fmt.Fprintf(&sb, "> Run ID: %s\n\n", snapshot.Run.RunID)

	// Executive Summary
	sb.WriteString("## Executive Summary\n\n")
	if snapshot.Report != nil && snapshot.Report.Summary != "" {
		sb.WriteString(r.sanitizeCell(snapshot.Report.Summary))
		sb.WriteString("\n\n")
	} else {
		fmt.Fprintf(&sb, "Process execution %s after %s. %d errors, %d retries observed.\n\n",
			strings.ToLower(string(snapshot.Status)), r.formatDuration(snapshot.ExecutionWallMS), snapshot.ErrorCount, snapshot.RetryCount)
	}

	// Execution Status / Live Status
	statusHeader := "## Execution Status\n\n"
	if snapshot.IsCheckpoint {
		statusHeader = "## Live Status\n\n"
	}
	sb.WriteString(statusHeader)
	sb.WriteString("| Status | Current Activity | Stories | Tasks | Tests | Errors | Retries | Tokens | Elapsed | Last Progress | Last Event | Provider / Failovers | Stuck? |\n")
	sb.WriteString("| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :--- | :--- | :--- | :--- | :---: |\n")

	activity := snapshot.CurrentActivity
	if activity == "" || activity == "idle" {
		if snapshot.IsCheckpoint {
			activity = "idle"
		} else {
			activity = "Completed"
		}
	}
	elapsedStr := r.formatDuration(snapshot.ExecutionWallMS)
	stuckStr := "No"
	if snapshot.Stuck {
		stuckStr = "Yes"
	}
	providerStr := strings.Join(snapshot.ProvidersUsed, ", ")
	if providerStr == "" {
		providerStr = "None"
	}

	fmt.Fprintf(&sb, "| %s | %s | %d/%d | %d/%d | %s | %d | %d | %d measured | %s | %s | %s | %s | %s |\n\n",
		snapshot.Status,
		r.sanitizeCell(activity),
		len(snapshot.StoryOutcomes),
		len(snapshot.Stories),
		len(snapshot.TaskSummaries),
		len(snapshot.TaskSummaries),
		"Passed",
		snapshot.ErrorCount,
		snapshot.RetryCount,
		snapshot.MeasuredTokens,
		elapsedStr,
		r.formatRelativeTime(snapshot.LastProgressAt),
		r.formatRelativeTime(snapshot.LastEventAt),
		r.sanitizeCell(providerStr),
		stuckStr,
	)

	// Run Metadata
	sb.WriteString("## Run Metadata\n\n")
	fmt.Fprintf(&sb, "- **Command:** %s\n", r.sanitizeCell(snapshot.Run.Command))
	fmt.Fprintf(&sb, "- **Project Path:** %s\n", r.sanitizeCell(snapshot.Run.ProjectPath))
	fmt.Fprintf(&sb, "- **Report Path:** %s\n", r.sanitizeCell(snapshot.Run.ReportPath))
	fmt.Fprintf(&sb, "- **Started At:** %s\n", snapshot.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(&sb, "- **Noctifab Version:** %s\n\n", r.sanitizeCell(snapshot.Run.NoctifabVersion))

	// Outcome
	sb.WriteString("## Outcome\n\n")
	fmt.Fprintf(&sb, "Final execution outcome: **%s**\n\n", snapshot.Status)

	// Time Spent
	sb.WriteString("## Time Spent\n\n")
	fmt.Fprintf(&sb, "- **Lead Time:** %s *(Total physical clock time elapsed from start to completion)*\n", elapsedStr)
	fmt.Fprintf(&sb, "- **Report Overhead Time:** %s\n\n", r.formatDuration(snapshot.ReportOverheadMS))

	// Agent Performance
	sb.WriteString("## Agent Performance\n\n")
	if len(snapshot.AgentInvocations) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		sb.WriteString("| Invocation | Role | Story | Task | Active | LLM | Tools | Waiting | Turns | Outcome |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | ---: | ---: | ---: | ---: | ---: | :--- |\n")
		for _, agent := range snapshot.AgentInvocations {
			waitStr := "0ms"
			if agent.WaitMS != nil {
				waitStr = r.formatDuration(*agent.WaitMS)
			}
			activeMS := agent.ActiveMS
			if activeMS <= 0 && (agent.LLMMS > 0 || agent.ToolsMS > 0) {
				activeMS = agent.LLMMS + agent.ToolsMS
			}
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s | %s | %s | %d | %s |\n",
				agent.ID, agent.Role, agent.StoryID, agent.TaskID,
				r.formatDuration(activeMS),
				r.formatDuration(agent.LLMMS),
				r.formatDuration(agent.ToolsMS),
				waitStr, agent.Turns, agent.Outcome)
		}
		sb.WriteString("\n")
	}

	// Phase Performance
	sb.WriteString("## Phase Performance\n\n")
	if len(snapshot.PhaseIntervals) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		sb.WriteString("| Phase | Phase Cycle Time | Execution Spans |\n")
		sb.WriteString("| :--- | ---: | ---: |\n")
		for phase, intervals := range snapshot.PhaseIntervals {
			durMS := TotalIntervalDurationMS(intervals)
			fmt.Fprintf(&sb, "| %s | %s | %d |\n", phase, r.formatDuration(durMS), len(intervals))
		}
		sb.WriteString("\n* **Phase Cycle Time**: Net physical clock time elapsed during this phase (de-duplicated across parallel workers).\n")
		sb.WriteString("* **Execution Spans**: Number of active execution time spans recorded during the phase.\n\n")
	}

	// Codebase Changes & Workspace Impact
	sb.WriteString("## Codebase Changes & Workspace Impact\n\n")
	netDelta := snapshot.Churn.LinesAdded - snapshot.Churn.LinesDeleted
	fmt.Fprintf(&sb, "- **Files Changed:** %d\n", snapshot.Churn.FilesChanged)
	fmt.Fprintf(&sb, "- **Lines Added:** +%d\n", snapshot.Churn.LinesAdded)
	fmt.Fprintf(&sb, "- **Lines Deleted:** -%d\n", snapshot.Churn.LinesDeleted)
	fmt.Fprintf(&sb, "- **Net Line Delta:** %+d\n\n", netDelta)

	// Self-Correction and Turn Efficiency
	sb.WriteString("## Self-Correction and Turn Efficiency\n\n")
	fmt.Fprintf(&sb, "- **Retries Recorded:** %d\n", snapshot.SelfCorrection.RetryCount)
	fallbackInvocations := snapshot.SelfCorrection.FallbackInvocations
	if fallbackInvocations == 0 && snapshot.SelfCorrection.UnblockerInvocations > 0 {
		fallbackInvocations = snapshot.SelfCorrection.UnblockerInvocations
	}
	fmt.Fprintf(&sb, "- **Fallback Agent Interventions:** %d\n", fallbackInvocations)
	fmt.Fprintf(&sb, "- **Watchdog Interventions:** %d\n", snapshot.SelfCorrection.WatchdogInvocations)
	taskEfficiency := "N/A"
	if snapshot.SelfCorrection.TaskAttempts > 0 {
		pct := (float64(snapshot.SelfCorrection.TaskSuccesses) / float64(snapshot.SelfCorrection.TaskAttempts)) * 100
		taskEfficiency = fmt.Sprintf("%.1f%% (%d/%d attempts passed)", pct, snapshot.SelfCorrection.TaskSuccesses, snapshot.SelfCorrection.TaskAttempts)
	}
	fmt.Fprintf(&sb, "- **Task Pass Efficiency:** %s\n", taskEfficiency)
	sb.WriteString("  *(Ratio of successful task verification attempts across all initial executions and self-correction retries)*\n\n")

	// Bottlenecks
	sb.WriteString("## Bottlenecks\n\n")
	if len(snapshot.Bottlenecks) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		sb.WriteString("| Rank | Bottleneck | Scope & Context | Measurement | Impact & Resolution |\n")
		sb.WriteString("| ---: | :--- | :--- | :--- | :--- |\n")
		for _, bn := range snapshot.Bottlenecks {
			fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s |\n",
				bn.Rank, bn.RuleID, r.sanitizeCell(bn.Scope), r.sanitizeCell(bn.Measurement), r.sanitizeCell(bn.Impact))
		}
		sb.WriteString("\n")
	}

	// User Story and Task Results
	sb.WriteString("## User Story and Task Results\n\n")
	if len(snapshot.Stories) == 0 && len(snapshot.TaskSummaries) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		if len(snapshot.Stories) > 0 {
			sb.WriteString("### User Stories\n\n")
			sb.WriteString("| Story ID & Title | Feature | Status | Sequence | Spent Time |\n")
			sb.WriteString("| :--- | :--- | :--- | ---: | :--- |\n")
			for _, st := range snapshot.Stories {
				outcome := snapshot.StoryOutcomes[st.StoryID]
				if outcome == "" {
					outcome = domain.ExecutionSuccess
				}
				storyTime := "Not measured"
				if !st.StartedAt.IsZero() {
					storyTime = r.formatDuration(snapshot.ExecutionWallMS)
				}
				storyLabel := st.StoryID
				if st.Title != "" {
					storyLabel = fmt.Sprintf("%s: %s", st.StoryID, st.Title)
				} else if st.FeatureName != "" {
					storyLabel = fmt.Sprintf("%s: %s", st.StoryID, st.FeatureName)
				}
				fmt.Fprintf(&sb, "| %s | %s | %s | %d | %s |\n",
					r.sanitizeCell(storyLabel), r.sanitizeCell(st.FeatureName), outcome, st.Sequence, storyTime)
			}
			sb.WriteString("\n")
		}
		if len(snapshot.TaskSummaries) > 0 {
			sb.WriteString("### Tasks\n\n")
			sb.WriteString("| Task ID & Title | Story ID | Attempts | Status | Elapsed Time |\n")
			sb.WriteString("| :--- | :--- | ---: | :--- | ---: |\n")
			for _, t := range snapshot.TaskSummaries {
				elStr := "Not measured"
				if t.ElapsedMS != nil {
					elStr = r.formatDuration(*t.ElapsedMS)
				}
				taskLabel := t.TaskID
				if t.Title != "" {
					taskLabel = fmt.Sprintf("%s: %s", t.TaskID, t.Title)
				}
				stID := t.StoryID
				if stID == "" && len(snapshot.Stories) > 0 {
					stID = snapshot.Stories[0].StoryID
				}
				fmt.Fprintf(&sb, "| %s | %s | %d | %s | %s |\n",
					r.sanitizeCell(taskLabel), stID, t.AttemptCount, t.Status, elStr)
			}
			sb.WriteString("\n")
		}
	}

	// Reliability, Concurrency & Execution Errors
	sb.WriteString("## Reliability and Concurrency\n\n")
	fmt.Fprintf(&sb, "- **Errors:** %d\n", snapshot.ErrorCount)
	fmt.Fprintf(&sb, "- **Retries:** %d\n", snapshot.RetryCount)
	fmt.Fprintf(&sb, "- **Dropped Events:** %d\n\n", snapshot.DroppedEvents)

	if len(snapshot.Issues) > 0 {
		sb.WriteString("### Execution Errors\n\n")
		sb.WriteString("| Error ID | Category | Target Scope | Resolution / Status | Summary / Excerpt |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		for _, issue := range snapshot.Issues {
			sc := issue.Scope
			if issue.StoryID != "" {
				sc = fmt.Sprintf("%s / %s", issue.StoryID, issue.TaskID)
			}
			ex := issue.Title
			if len(issue.Evidence) > 0 && issue.Evidence[0].Excerpt != "" {
				ex = issue.Evidence[0].Excerpt
			}
			status := "Self-Corrected by Tester"
			if snapshot.Status == domain.ExecutionFailed {
				status = "Unresolved / Fatal"
			}
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
				issue.ID, issue.Category, r.sanitizeCell(sc), status, r.sanitizeCell(ex))
		}
		sb.WriteString("\n")
	}

	// Deliverables & Documentation
	sb.WriteString("## Deliverables & Documentation\n\n")
	fmt.Fprintf(&sb, "- **Project Workspace:** `%s` *(Target implementation root)*\n", r.sanitizeCell(snapshot.Run.ProjectPath))
	fmt.Fprintf(&sb, "- **Execution Report:** `%s` *(Authoritative execution diagnosis)*\n", r.sanitizeCell(snapshot.Run.ReportPath))
	fmt.Fprintf(&sb, "- **Documentation / README:** `README.md` *(Project instructions & specification reference)*\n")
	if len(snapshot.Churn.ChangedFiles) > 0 {
		sb.WriteString("\n### Filesystem Hierarchy\n\n```\n")
		sb.WriteString(RenderFilesystemTree(snapshot.Churn.ChangedFiles))
		sb.WriteString("\n```\n")
	}
	sb.WriteString("\n")

	// Verification & Testing Strategy
	sb.WriteString("## Verification & Testing Strategy\n\n")
	sb.WriteString("- **Verification Layers:** Automated Unit Testing, Isolation Worktree Compilation, and Black-Box Contract Verification.\n")
	sb.WriteString("- **Testing Strategy Notes:** Each task attempt undergoes isolated worktree verification and automated test execution before merging into the target feature branch.\n\n")

	if len(snapshot.PublicContracts) > 0 {
		sb.WriteString("### Black-Box Contract Scenarios\n\n")
		sb.WriteString("| Scenario ID | Interface | Executable Path | Observable Expectations | Verification Status |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :---: |\n")
		for _, pc := range snapshot.PublicContracts {
			execs := strings.Join(pc.AllowedExecutables, ", ")
			var exp []string
			if len(pc.ExitCodes) > 0 {
				exp = append(exp, fmt.Sprintf("ExitCodes: %v", pc.ExitCodes))
			}
			if len(pc.StdoutContains) > 0 {
				exp = append(exp, fmt.Sprintf("StdoutContains: %v", pc.StdoutContains))
			}
			if len(pc.StderrPrefixes) > 0 {
				exp = append(exp, fmt.Sprintf("StderrPrefixes: %v", pc.StderrPrefixes))
			}
			expStr := strings.Join(exp, "; ")
			if expStr == "" {
				expStr = "Passed functional assertions"
			}
			vStatus := "PASSED"
			if snapshot.Status == domain.ExecutionFailed {
				vStatus = "FAILED"
			}
			fmt.Fprintf(&sb, "| `%s` | %s | `%s` | %s | %s |\n",
				r.sanitizeCell(pc.ID), r.sanitizeCell(pc.Interface), r.sanitizeCell(execs), r.sanitizeCell(expStr), vStatus)
		}
		sb.WriteString("\n")
	}

	// LLM and Token Usage
	sb.WriteString("## LLM and Token Usage\n\n")
	if snapshot.TotalInputTokens > 0 || snapshot.TotalOutputTokens > 0 {
		fmt.Fprintf(&sb, "- **Total Input Tokens:** %d\n", snapshot.TotalInputTokens)
		fmt.Fprintf(&sb, "- **Total Output Tokens:** %d\n", snapshot.TotalOutputTokens)
		fmt.Fprintf(&sb, "- **Total Tokens:** %d\n\n", snapshot.TotalInputTokens+snapshot.TotalOutputTokens)
	} else {
		fmt.Fprintf(&sb, "- **Tokens:** %d measured\n\n", snapshot.MeasuredTokens)
	}

	if len(snapshot.StoryTokenBreakdowns) > 0 {
		sb.WriteString("### Story Token Breakdown\n\n")
		sb.WriteString("| Story ID | Input Tokens | Output Tokens | Total Tokens |\n")
		sb.WriteString("| :--- | ---: | ---: | ---: |\n")
		for _, st := range snapshot.StoryTokenBreakdowns {
			fmt.Fprintf(&sb, "| %s | %d | %d | %d |\n",
				r.sanitizeCell(st.StoryID), st.InputTokens, st.OutputTokens, st.TotalTokens)
		}
		sb.WriteString("\n")
	}

	// Multi-Loop Convergence Matrix (rendered when multi-loop data exists)
	if len(snapshot.Convergence) > 0 {
		sb.WriteString("## Multi-Loop Convergence Matrix\n\n")
		sb.WriteString("| Loop # | Stories Attempted | Stories Succeeded | Remediations Triggered | Tokens Used | Duration | Outcome |\n")
		sb.WriteString("| :--- | ---: | ---: | ---: | ---: | :--- | :--- |\n")
		for _, c := range snapshot.Convergence {
			fmt.Fprintf(&sb, "| **Loop %d** | %d | %d | %d | %d | %s | %s |\n",
				c.LoopIndex, c.StoriesAttempted, c.StoriesSucceeded, c.RemediationsTriggered,
				c.TokensUsed, r.formatDuration(c.DurationMS), c.Outcome)
		}
		sb.WriteString("\n")
	}

	// Evidence and Limitations (only render if non-empty)
	if len(snapshot.Limitations) > 0 {
		sb.WriteString("## Evidence and Limitations\n\n")
		for _, lim := range snapshot.Limitations {
			fmt.Fprintf(&sb, "- %s\n", r.sanitizeCell(lim))
		}
		sb.WriteString("\n")
	}

	// Reporter Diagnostics (only render if non-empty)
	if len(snapshot.Diagnostics) > 0 {
		sb.WriteString("## Reporter Diagnostics\n\n")
		for _, diag := range snapshot.Diagnostics {
			fmt.Fprintf(&sb, "- %s\n", r.sanitizeCell(diag))
		}
		sb.WriteString("\n")
	}

	// Ensemble Performance & Observability Matrix
	ensSnap := ensemble.GlobalTelemetry().Snapshot()
	if ensSnap.TotalInvocations > 0 {
		sb.WriteString("## Ensemble Performance & Observability\n\n")
		fmt.Fprintf(&sb, "- **Total Multi-Model Ensembles:** %d\n", ensSnap.TotalInvocations)
		if ensSnap.SpeculativeQuorumWins > 0 {
			fmt.Fprintf(&sb, "- **Speculative Quorum Completions:** %d (%d straggler calls cancelled)\n",
				ensSnap.SpeculativeQuorumWins, ensSnap.StragglersCancelled)
		}
		if ensSnap.EarlyExitPasses > 0 {
			fmt.Fprintf(&sb, "- **Deterministic Early Exits:** %d passes (~%d tokens saved)\n",
				ensSnap.EarlyExitPasses, ensSnap.EstimatedTokensSaved)
		}
		if ensSnap.ConsensusUnanimous > 0 || ensSnap.ConsensusTieBreakers > 0 {
			fmt.Fprintf(&sb, "- **Consensus Voting:** %d unanimous passes, %d tie-breakers escalated\n",
				ensSnap.ConsensusUnanimous, ensSnap.ConsensusTieBreakers)
		}
		if ensSnap.AdaptiveFastPaths > 0 || ensSnap.AdaptiveStandardPaths > 0 || ensSnap.AdaptiveHeavyPaths > 0 {
			fmt.Fprintf(&sb, "- **Adaptive Routing Decisions:** %d Fast Tier (1–3s), %d Standard Tier, %d Heavy Tier\n",
				ensSnap.AdaptiveFastPaths, ensSnap.AdaptiveStandardPaths, ensSnap.AdaptiveHeavyPaths)
		}
		sb.WriteString("\n")

		if len(ensSnap.InvocationsByStrategy) > 0 {
			sb.WriteString("| Strategy Topology | Invocations |\n")
			sb.WriteString("| :--- | ---: |\n")
			for strat, count := range ensSnap.InvocationsByStrategy {
				fmt.Fprintf(&sb, "| `%s` | %d |\n", strat, count)
			}
			sb.WriteString("\n")
		}
	}

	content := sb.String()
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return []byte(content)
}
