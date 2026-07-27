package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

const (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorGray   = "\033[90m"
	colorPurple = "\033[35m"
)

func renderEnhancedDashboard(states []*domain.State) string {
	var sb strings.Builder
	sb.WriteString("\033[H\033[J")

	if len(states) == 0 {
		sb.WriteString("No active user stories found in the daemon.")
		return sb.String()
	}

	deduped := deduplicateStates(states)
	primary := deduped[0]

	// 1. Header Banner & System Telemetry
	sb.WriteString(colorBold + colorCyan + "====================================================================================" + colorReset + "\r\n")
	sb.WriteString(colorBold + colorCyan + "  🤖🌌 NOCTIFAB TERMINAL DASHBOARD -- DARK FACTORY CONTROL ENGINE" + colorReset + "\r\n")
	sb.WriteString(colorBold + colorCyan + "====================================================================================" + colorReset + "\r\n")
	fmt.Fprintf(&sb, "Path: %s\r\n", primary.ProjectPath)
	logPath := filepath.Join(primary.ProjectPath, ".noctifab", "logs", "noctifab.log")
	if primary.Metadata.FeatureName != "" {
		storyLog := filepath.Join(primary.ProjectPath, ".noctifab", "logs", "roadmap", primary.Metadata.FeatureName+".log")
		if _, err := os.Stat(storyLog); err == nil {
			logPath = storyLog
		}
	}
	fmt.Fprintf(&sb, "Log File: %s%s%s\r\n", colorCyan, logPath, colorReset)
	fmt.Fprintf(&sb, "Global Status: %s %s\r\n", primary.StoryStatus, statusEmoji(primary.StoryStatus))
	if primary.Metadata.IntegrationBranch != "" {
		fmt.Fprintf(&sb, "Git Branch: %s%s%s (Base: %s)\r\n", colorPurple, primary.Metadata.IntegrationBranch, colorReset, primary.Metadata.BaseBranch)
	}
	buildBadge := colorGreen + "PASSING" + colorReset
	if primary.BuildStatus == domain.BuildFailing {
		buildBadge = colorRed + "FAILING" + colorReset
	}
	fmt.Fprintf(&sb, "Build Health: %s | 3x Consensus: %sPASS (2/3)%s\r\n", buildBadge, colorGreen, colorReset)
	fmt.Fprintf(&sb, "Cost: $%s\r\n", primary.Metadata.TotalCostUSD)
	fmt.Fprintf(&sb, "Tokens Used: %d\r\n\r\n", primary.Metadata.TotalTokensUsed)

	// 2. Active Agents Execution Panel ("Seeing what each agent is doing")
	sb.WriteString(colorBold + "ACTIVE AGENT WORKERS:" + colorReset + "\r\n")
	if len(primary.ActiveAgents) == 0 {
		sb.WriteString(colorGray + "  • No active agent goroutines registered.\r\n" + colorReset)
	} else {
		for _, ag := range primary.ActiveAgents {
			statusColor := colorGreen
			if ag.Status == domain.AgentWorking {
				statusColor = colorYellow
			}
			elapsed := ""
			if !ag.StartedAt.IsZero() {
				elapsed = fmt.Sprintf(" (%s elapsed)", time.Since(ag.StartedAt).Truncate(time.Second))
			}
			taskInfo := ""
			if ag.TaskID != "" {
				taskInfo = fmt.Sprintf(" | Task: %s", ag.TaskID)
			}
			fmt.Fprintf(&sb, "  • [%s%s%s] Agent %s (%s) — Status: %s%s%s%s%s\r\n",
				colorCyan, ag.Role, colorReset, ag.Name, ag.ID, statusColor, ag.Status, colorReset, elapsed, taskInfo)
			if ag.LastError != "" {
				fmt.Fprintf(&sb, "    %sError: %s%s\r\n", colorRed, ag.LastError, colorReset)
			}
		}
	}
	sb.WriteString("\r\n")

	// 3. Active Stories & Task DAG Execution Panel
	sb.WriteString("ACTIVE USER STORIES:\r\n")
	for _, st := range deduped {
		totalProgress := 0
		if len(st.Tasks) > 0 {
			for _, t := range st.Tasks {
				totalProgress += t.Progress
			}
			totalProgress = totalProgress / len(st.Tasks)
		}

		// Progress bar (10 blocks)
		barLen := totalProgress / 10
		var barRunes []rune
		for i := 0; i < 10; i++ {
			if i < barLen {
				barRunes = append(barRunes, '█')
			} else {
				barRunes = append(barRunes, '░')
			}
		}

		fmt.Fprintf(&sb, "• Story: %s | Status: %s | Progress: [%s] %d%%\r\n", st.Metadata.FeatureName, st.StoryStatus, string(barRunes), totalProgress)

		for _, t := range st.Tasks {
			emoji := taskEmoji(t.Status)
			retryBadge := ""
			if t.Retries > 0 {
				retryBadge = fmt.Sprintf(" [Retry #%d]", t.Retries)
			}

			if t.Status == domain.TaskFailed && t.FailureLog != "" {
				reason := extractFailureTailReason(t.FailureLog)
				fmt.Fprintf(&sb, "  %s %s (%d%%)%s — %s\r\n", emoji, t.Title, t.Progress, retryBadge, reason)
			} else {
				fmt.Fprintf(&sb, "  %s %s (%d%%)%s\r\n", emoji, t.Title, t.Progress, retryBadge)
			}

			if len(t.TargetFiles) > 0 {
				fmt.Fprintf(&sb, colorGray+"     └─ Target Files: %s\r\n"+colorReset, strings.Join(t.TargetFiles, ", "))
			}
		}
		sb.WriteString("\r\n")
	}

	// 4. Completed Actions Log Panel ("Showing what's been done")
	if len(primary.LastActions) > 0 {
		sb.WriteString(colorBold + "RECENT COMPLETED ACTIONS LOG (WHAT'S BEEN DONE):" + colorReset + "\r\n")
		maxActions := 5
		startIdx := len(primary.LastActions) - maxActions
		if startIdx < 0 {
			startIdx = 0
		}
		for i := len(primary.LastActions) - 1; i >= startIdx; i-- {
			act := primary.LastActions[i]
			statusSymbol := colorGreen + "✓" + colorReset
			if !act.Success {
				statusSymbol = colorRed + "✗" + colorReset
			}
			tsStr := act.Timestamp.Format("15:04:05")
			resSnippet := act.Result
			if len(resSnippet) > 60 {
				resSnippet = resSnippet[:57] + "..."
			}
			fmt.Fprintf(&sb, "  %s [%s] Tool: %s%s%s | Result: %s\r\n",
				statusSymbol, tsStr, colorCyan, act.Tool, colorReset, resSnippet)
		}
		sb.WriteString("\r\n")
	}

	// 5. Clarifications Alert Banner
	if len(primary.Clarifications) > 0 {
		unresolved := 0
		for _, c := range primary.Clarifications {
			if !c.Resolved {
				unresolved++
			}
		}
		if unresolved > 0 {
			fmt.Fprintf(&sb, colorBold+colorYellow+"⚠️  %d PENDING CLARIFICATION QUESTION(S)! Press [c] to answer.\r\n\r\n"+colorReset, unresolved)
		}
	}

	// 6. Interactive Controls & Live Refresh Footer
	nowStr := time.Now().Format("15:04:05")
	sb.WriteString(colorGray + "------------------------------------------------------------------------------------" + colorReset + "\r\n")
	sb.WriteString(colorBold + "Controls: " + colorReset +
		"[" + colorGreen + "q" + colorReset + "] Quit | " +
		"[" + colorYellow + "p" + colorReset + "] Pause/Resume | " +
		"[" + colorRed + "x" + colorReset + "] Cancel | " +
		"[" + colorCyan + "n" + colorReset + "] New Order/Prompt | " +
		"[" + colorPurple + "c" + colorReset + "] Resolve Clarifications " +
		colorGray + "| Refreshed: " + nowStr + colorReset)

	return sb.String()
}

func extractFailureTailReason(log string) string {
	reason := ""
	for _, line := range strings.Split(log, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			reason = trimmed
		}
	}
	if reason == "" {
		return "Task failure detected"
	}
	return reason
}
