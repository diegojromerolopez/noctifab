package reporting

import (
	"fmt"
	"strings"
	"time"
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

	checkpointStr := "no"
	if snapshot.IsCheckpoint {
		checkpointStr = "yes"
	}

	sb.WriteString("# Noctifab Execution Report\n\n")
	fmt.Fprintf(&sb, "> Status: %s\n", snapshot.Status)
	fmt.Fprintf(&sb, "> Run ID: %s\n", snapshot.Run.RunID)
	fmt.Fprintf(&sb, "> Checkpoint: %s\n\n", checkpointStr)

	// Executive Summary
	sb.WriteString("## Executive Summary\n\n")
	if snapshot.Report != nil && snapshot.Report.Summary != "" {
		sb.WriteString(r.sanitizeCell(snapshot.Report.Summary) + "\n\n")
	} else {
		fmt.Fprintf(&sb, "Process execution %s after %d ms. %d errors, %d retries observed.\n\n",
			strings.ToLower(string(snapshot.Status)), snapshot.ExecutionWallMS, snapshot.ErrorCount, snapshot.RetryCount)
	}

	// Live Status
	sb.WriteString("## Live Status\n\n")
	sb.WriteString("| Status | Current Activity | Stories | Tasks | Tests | Errors | Retries | Tokens | Elapsed | Last Progress | Last Event | Provider / Failovers | Stuck? |\n")
	sb.WriteString("| :--- | :--- | ---: | ---: | ---: | ---: | ---: | ---: | :--- | :--- | :--- | :--- | :---: |\n")

	activity := snapshot.CurrentActivity
	if activity == "" {
		activity = "idle"
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
		"Not measured",
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
	fmt.Fprintf(&sb, "- **Execution Wall Time:** %d ms (%s)\n", snapshot.ExecutionWallMS, elapsedStr)
	fmt.Fprintf(&sb, "- **Report Overhead Time:** %d ms\n\n", snapshot.ReportOverheadMS)

	// Agent Performance
	sb.WriteString("## Agent Performance\n\n")
	if len(snapshot.AgentInvocations) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		sb.WriteString("| Invocation | Role | Story | Task | Active | LLM | Tools | Waiting | Turns | Outcome |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | ---: | ---: | ---: | ---: | ---: | :--- |\n")
		for _, agent := range snapshot.AgentInvocations {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %d ms | %d ms | %d ms | %s | %d | %s |\n",
				agent.ID, agent.Role, agent.StoryID, agent.TaskID,
				agent.ActiveMS, agent.LLMMS, agent.ToolsMS,
				"Not measured", agent.Turns, agent.Outcome)
		}
		sb.WriteString("\n")
	}

	// Phase Performance
	sb.WriteString("## Phase Performance\n\n")
	if len(snapshot.PhaseIntervals) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		sb.WriteString("| Phase | Union Wall Time | Merged Intervals |\n")
		sb.WriteString("| :--- | ---: | ---: |\n")
		for phase, intervals := range snapshot.PhaseIntervals {
			durMS := TotalIntervalDurationMS(intervals)
			fmt.Fprintf(&sb, "| %s | %d ms | %d |\n", phase, durMS, len(intervals))
		}
		sb.WriteString("\n")
	}

	// Code Churn and Workspace Impact
	sb.WriteString("## Code Churn and Workspace Impact\n\n")
	sb.WriteString("Not measured\n\n")

	// Self-Correction and Turn Efficiency
	sb.WriteString("## Self-Correction and Turn Efficiency\n\n")
	sb.WriteString("Not measured\n\n")

	// Bottlenecks
	sb.WriteString("## Bottlenecks\n\n")
	if len(snapshot.Bottlenecks) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		sb.WriteString("| Rank | Rule | Scope | Measurement | Impact |\n")
		sb.WriteString("| ---: | :--- | :--- | :--- | :--- |\n")
		for _, bn := range snapshot.Bottlenecks {
			fmt.Fprintf(&sb, "| %d | %s | %s | %s | %s |\n",
				bn.Rank, bn.RuleID, r.sanitizeCell(bn.Scope), r.sanitizeCell(bn.Measurement), r.sanitizeCell(bn.Impact))
		}
		sb.WriteString("\n")
	}

	// Issues Found
	sb.WriteString("## Issues Found\n\n")
	if len(snapshot.Issues) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		sb.WriteString("| ID | Kind | Category | Severity | Title | Impact |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- | :--- |\n")
		for _, issue := range snapshot.Issues {
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s | %s |\n",
				issue.ID, issue.Kind, issue.Category, issue.Severity, r.sanitizeCell(issue.Title), r.sanitizeCell(issue.Impact))
		}
		sb.WriteString("\n")
	}

	// Proposals and Next Actions
	sb.WriteString("## Proposals and Next Actions\n\n")
	if len(snapshot.Proposals) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		sb.WriteString("| ID | Issues | Scope | Recommendation | Verification |\n")
		sb.WriteString("| :--- | :--- | :--- | :--- | :--- |\n")
		for _, prop := range snapshot.Proposals {
			issuesJoined := strings.Join(prop.IssueIDs, ", ")
			fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n",
				prop.ID, issuesJoined, prop.Scope, r.sanitizeCell(prop.Action), r.sanitizeCell(prop.Verification))
		}
		sb.WriteString("\n")
	}

	// User Story and Task Results
	sb.WriteString("## User Story and Task Results\n\n")
	if len(snapshot.TaskSummaries) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		sb.WriteString("| Task ID | Story ID | Attempts | Status | Elapsed |\n")
		sb.WriteString("| :--- | :--- | ---: | :--- | ---: |\n")
		for _, t := range snapshot.TaskSummaries {
			elStr := "Not measured"
			if t.ElapsedMS != nil {
				elStr = fmt.Sprintf("%d ms", *t.ElapsedMS)
			}
			fmt.Fprintf(&sb, "| %s | %s | %d | %s | %s |\n",
				t.TaskID, t.StoryID, t.AttemptCount, t.Status, elStr)
		}
		sb.WriteString("\n")
	}

	// LLM, Token, and Cost Usage
	sb.WriteString("## LLM, Token, and Cost Usage\n\n")
	fmt.Fprintf(&sb, "- **Measured Tokens:** %d\n", snapshot.MeasuredTokens)
	fmt.Fprintf(&sb, "- **Estimated Tokens:** %d\n", snapshot.EstimatedTokens)
	costStr := snapshot.TotalCostUSD
	if costStr == "" {
		costStr = "Not measured"
	}
	fmt.Fprintf(&sb, "- **Total Cost USD:** %s\n\n", costStr)

	// Reliability and Concurrency
	sb.WriteString("## Reliability and Concurrency\n\n")
	fmt.Fprintf(&sb, "- **Errors:** %d\n", snapshot.ErrorCount)
	fmt.Fprintf(&sb, "- **Retries:** %d\n", snapshot.RetryCount)
	fmt.Fprintf(&sb, "- **Dropped Events:** %d\n\n", snapshot.DroppedEvents)

	// Evidence and Limitations
	sb.WriteString("## Evidence and Limitations\n\n")
	if len(snapshot.Limitations) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		for _, lim := range snapshot.Limitations {
			fmt.Fprintf(&sb, "- %s\n", r.sanitizeCell(lim))
		}
		sb.WriteString("\n")
	}

	// Reporter Diagnostics
	sb.WriteString("## Reporter Diagnostics\n\n")
	if len(snapshot.Diagnostics) == 0 {
		sb.WriteString("None observed\n\n")
	} else {
		for _, diag := range snapshot.Diagnostics {
			fmt.Fprintf(&sb, "- %s\n", r.sanitizeCell(diag))
		}
		sb.WriteString("\n")
	}

	content := sb.String()
	// Ensure UTF-8 clean line ending
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return []byte(content)
}

func (r *Renderer) sanitizeCell(s string) string {
	s = r.redactor.Redact(s)
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}

func (r *Renderer) formatDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	return fmt.Sprintf("%02dm %02ds (%d ms)", m, s, ms)
}

func (r *Renderer) formatRelativeTime(t time.Time) string {
	if t.IsZero() {
		return "Never"
	}
	diff := time.Since(t).Truncate(time.Second)
	if diff < 0 {
		diff = 0
	}
	return fmt.Sprintf("%s ago", diff.String())
}
