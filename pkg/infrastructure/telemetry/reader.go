package telemetry

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SpanSummary represents a summarized span record for LLM context prompts.
type SpanSummary struct {
	Name       string         `json:"name"`
	Status     string         `json:"status"`
	DurationMS int64          `json:"duration_ms"`
	StartTime  string         `json:"start_time"`
	Attributes map[string]any `json:"attributes"`
}

// ReadRecentSpans parses the last maxSpans trace records from .noctifab/traces.jsonl
// (or .noctifab/telemetry/spans.jsonl) under projectPath.
func ReadRecentSpans(projectPath string, maxSpans int) ([]SpanSummary, error) {
	if maxSpans <= 0 {
		maxSpans = 15
	}

	paths := []string{
		filepath.Join(projectPath, ".noctifab", "traces.jsonl"),
		filepath.Join(projectPath, ".noctifab", "telemetry", "spans.jsonl"),
	}

	var traceFile string
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Size() > 0 {
			traceFile = p
			break
		}
	}

	if traceFile == "" {
		return nil, nil
	}

	f, err := os.Open(traceFile)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var lines []string
	scanner := bufio.NewScanner(f)
	// Bounded ring buffer for the last lines
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			lines = append(lines, text)
			if len(lines) > maxSpans*2 {
				lines = lines[len(lines)-maxSpans:]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var spans []SpanSummary
	startIdx := 0
	if len(lines) > maxSpans {
		startIdx = len(lines) - maxSpans
	}

	for i := startIdx; i < len(lines); i++ {
		var s SpanSummary
		if err := json.Unmarshal([]byte(lines[i]), &s); err == nil {
			spans = append(spans, s)
		}
	}

	return spans, nil
}

// FormatSpansForPrompt formats recent spans into a concise diagnostic block for the LLM.
func FormatSpansForPrompt(spans []SpanSummary) string {
	if len(spans) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("#### 📡 Recent OpenTelemetry Workflow & Subprocess Spans:\n")
	sb.WriteString("| Span Name | Status | Duration | Key Attributes |\n")
	sb.WriteString("|:---|:---|:---|:---|\n")

	for _, s := range spans {
		var attrParts []string
		for k, v := range s.Attributes {
			if k == "command" || k == "task.id" || k == "story.id" || k == "error" || k == "action" || k == "package" {
				valStr := fmt.Sprintf("%v", v)
				if len(valStr) > 60 {
					valStr = valStr[:60] + "..."
				}
				attrParts = append(attrParts, fmt.Sprintf("`%s=%s`", k, valStr))
			}
		}
		attrSummary := strings.Join(attrParts, ", ")
		if attrSummary == "" {
			attrSummary = "-"
		}

		status := s.Status
		if status == "" || status == "Unset" {
			status = "OK"
		}

		fmt.Fprintf(&sb, "| `%s` | %s | %dms | %s |\n", s.Name, status, s.DurationMS, attrSummary)
	}

	return sb.String()
}
