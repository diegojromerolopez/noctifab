package services

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// SpecRenderer provides terminal formatting, colored diffs, and interactive prompts.
type SpecRenderer struct {
	in  io.Reader
	out io.Writer
}

// NewSpecRenderer instantiates a SpecRenderer with standard stdin/stdout.
func NewSpecRenderer() *SpecRenderer {
	return &SpecRenderer{
		in:  os.Stdin,
		out: os.Stdout,
	}
}

// NewCustomSpecRenderer instantiates a SpecRenderer with custom streams (for testing).
func NewCustomSpecRenderer(in io.Reader, out io.Writer) *SpecRenderer {
	return &SpecRenderer{
		in:  in,
		out: out,
	}
}

// PrintHeader prints a styled session banner.
func (r *SpecRenderer) PrintHeader(title string) {
	_, _ = fmt.Fprintf(r.out, "\n╭──────────────────────────────────────────────────────────────────────────────╮\n")
	_, _ = fmt.Fprintf(r.out, "│ 🌌 %-73s │\n", title)
	_, _ = fmt.Fprintf(r.out, "╰──────────────────────────────────────────────────────────────────────────────╯\n\n")
}

// PrintSuccess prints a success message with checkmark.
func (r *SpecRenderer) PrintSuccess(msg string) {
	_, _ = fmt.Fprintf(r.out, "✔ %s\n", msg)
}

// PrintInfo prints an informational progress line.
func (r *SpecRenderer) PrintInfo(msg string) {
	_, _ = fmt.Fprintf(r.out, "ℹ %s\n", msg)
}

// PrintError prints an error message.
func (r *SpecRenderer) PrintError(msg string) {
	_, _ = fmt.Fprintf(r.out, "✖ %s\n", msg)
}

// PrintApprovalMessage prints the detected termination reasoning.
func (r *SpecRenderer) PrintApprovalMessage(input, reason string) {
	_, _ = fmt.Fprintf(r.out, "\n✔ Approval intent detected (%q)\n", input)
	if reason != "" {
		_, _ = fmt.Fprintf(r.out, "  Reason: %s\n", reason)
	}
}

// RenderSpecPreview outputs the rendered specification markdown preview.
func (r *SpecRenderer) RenderSpecPreview(content string, turn int) {
	lines := strings.Split(content, "\n")
	_, _ = fmt.Fprintf(r.out, "\n────────────────────────────────────────────────────────────────────────────────\n")
	_, _ = fmt.Fprintf(r.out, "📄 Current SPEC.md Draft (Revision %d | %d lines)\n", turn, len(lines))
	_, _ = fmt.Fprintf(r.out, "────────────────────────────────────────────────────────────────────────────────\n")

	// If short, print full content; if very long, print first 40 lines and tail
	if len(lines) <= 60 {
		_, _ = fmt.Fprintln(r.out, content)
	} else {
		for i := 0; i < 35; i++ {
			_, _ = fmt.Fprintln(r.out, lines[i])
		}
		_, _ = fmt.Fprintf(r.out, "\n... [%d lines omitted for terminal preview] ...\n\n", len(lines)-50)
		for i := len(lines) - 15; i < len(lines); i++ {
			_, _ = fmt.Fprintln(r.out, lines[i])
		}
	}
	_, _ = fmt.Fprintf(r.out, "────────────────────────────────────────────────────────────────────────────────\n")
}

// CalculateDiff produces a simple line-by-line diff string between oldContent and newContent.
func (r *SpecRenderer) CalculateDiff(oldContent, newContent string) string {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	oldSet := make(map[string]bool)
	for _, l := range oldLines {
		if strings.TrimSpace(l) != "" {
			oldSet[l] = true
		}
	}

	newSet := make(map[string]bool)
	for _, l := range newLines {
		if strings.TrimSpace(l) != "" {
			newSet[l] = true
		}
	}

	var sb strings.Builder
	for _, l := range newLines {
		if strings.TrimSpace(l) != "" && !oldSet[l] {
			sb.WriteString("+ ")
			sb.WriteString(l)
			sb.WriteString("\n")
		}
	}
	for _, l := range oldLines {
		if strings.TrimSpace(l) != "" && !newSet[l] {
			sb.WriteString("- ")
			sb.WriteString(l)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// RenderDiff prints the diff with color / prefix.
func (r *SpecRenderer) RenderDiff(diff string) {
	if strings.TrimSpace(diff) == "" {
		return
	}
	_, _ = fmt.Fprintf(r.out, "\n📝 Specification Changes Applied:\n")
	lines := strings.Split(strings.TrimSpace(diff), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+ ") {
			_, _ = fmt.Fprintf(r.out, "  \033[32m%s\033[0m\n", line)
		} else if strings.HasPrefix(line, "- ") {
			_, _ = fmt.Fprintf(r.out, "  \033[31m%s\033[0m\n", line)
		} else {
			_, _ = fmt.Fprintf(r.out, "  %s\n", line)
		}
	}
	_, _ = fmt.Fprintln(r.out)
}

// PromptUserFeedback asks the human for feedback or completion command.
func (r *SpecRenderer) PromptUserFeedback(turn int) (string, error) {
	_, _ = fmt.Fprintf(r.out, "\n[Turn %d] What would you like to improve, fix, or add?\n", turn)
	_, _ = fmt.Fprintf(r.out, "(Enter your instructions, or say 'looks good' / 'stop' to approve)\n> ")

	reader := bufio.NewReader(r.in)
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// PromptYesNo prompts the user with a yes/no question.
func (r *SpecRenderer) PromptYesNo(question string, defaultYes bool) bool {
	hint := "[Y/n]"
	if !defaultYes {
		hint = "[y/N]"
	}
	_, _ = fmt.Fprintf(r.out, "\n? %s %s: ", question, hint)

	reader := bufio.NewReader(r.in)
	line, err := reader.ReadString('\n')
	if err != nil {
		return defaultYes
	}
	trimmed := strings.ToLower(strings.TrimSpace(line))
	if trimmed == "" {
		return defaultYes
	}
	return trimmed == "y" || trimmed == "yes"
}

// RenderHistory prints a formatted timeline of all revisions in the session.
func (r *SpecRenderer) RenderHistory(revisions []domain.SpecRevision, activeIdx int) {
	_, _ = fmt.Fprintf(r.out, "\n📜 Specification Revision Timeline:\n")
	for i, rev := range revisions {
		marker := " "
		if i == activeIdx {
			marker = "➔"
		}
		promptSummary := rev.Prompt
		if len(promptSummary) > 60 {
			promptSummary = promptSummary[:57] + "..."
		}
		if promptSummary == "" {
			promptSummary = "(Initial Pass)"
		}
		lineCount := len(strings.Split(rev.Content, "\n"))
		_, _ = fmt.Fprintf(r.out, "  %s v%d: [%d lines] %s (%s)\n", marker, rev.Version, lineCount, promptSummary, rev.Kind)
	}
	_, _ = fmt.Fprintln(r.out)
}

// RenderRollback prints confirmation of an undo / rollback operation.
func (r *SpecRenderer) RenderRollback(version int, lines int) {
	_, _ = fmt.Fprintf(r.out, "\n⏪ Rolled back to Revision %d (SPEC.v%d.md | %d lines)\n", version, version, lines)
	_, _ = fmt.Fprintf(r.out, "✔ Restored specification from snapshot cache (0 tokens used)\n")
}

// RenderCheckout prints confirmation of a checkout operation.
func (r *SpecRenderer) RenderCheckout(version int, lines int) {
	_, _ = fmt.Fprintf(r.out, "\n🎯 Checked out Revision %d (SPEC.v%d.md | %d lines)\n", version, version, lines)
	_, _ = fmt.Fprintf(r.out, "✔ Restored specification from snapshot cache (0 tokens used)\n")
}
