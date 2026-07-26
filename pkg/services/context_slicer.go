package services

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

// ContextSlicer prepares token-optimized prompt file contexts according to config mode.
type ContextSlicer struct {
	mode        config.ContextMode
	windowLines int
}

// NewContextSlicer initializes a ContextSlicer for the given mode and window settings.
func NewContextSlicer(cfg config.ContextConfig) *ContextSlicer {
	window := cfg.DiffWindowLines
	if window <= 0 {
		window = 15
	}
	return &ContextSlicer{
		mode:        cfg.GetMode(),
		windowLines: window,
	}
}

// SliceFileContext transforms raw file content based on the configured slicing mode.
func (s *ContextSlicer) SliceFileContext(relPath string, rawContent string, diffContent string) string {
	if s == nil {
		return formatFullFile(relPath, rawContent)
	}

	switch s.mode {
	case config.ContextModeDiffWindow:
		return s.formatDiffWindow(relPath, rawContent, diffContent)
	case config.ContextModeTreeSitter:
		return s.formatTreeSitterSymbols(relPath, rawContent)
	default:
		return formatFullFile(relPath, rawContent)
	}
}

func formatFullFile(relPath string, content string) string {
	return fmt.Sprintf("File %s:\n```\n%s\n```", relPath, content)
}

func (s *ContextSlicer) formatDiffWindow(relPath string, rawContent string, diffContent string) string {
	if strings.TrimSpace(diffContent) != "" {
		return fmt.Sprintf("File %s (Diff Window +/- %d lines):\n```diff\n%s\n```", relPath, s.windowLines, diffContent)
	}

	lines := strings.Split(rawContent, "\n")
	if len(lines) <= s.windowLines*2 {
		return formatFullFile(relPath, rawContent)
	}

	// Sliced window: head lines + snippet indicator + tail lines
	head := strings.Join(lines[:s.windowLines], "\n")
	tail := strings.Join(lines[len(lines)-s.windowLines:], "\n")
	return fmt.Sprintf("File %s (Window Sliced %d lines):\n```\n%s\n\n... [%d lines omitted] ...\n\n%s\n```",
		relPath, s.windowLines, head, len(lines)-s.windowLines*2, tail)
}

var (
	// Language-agnostic regex matching function signatures, class/struct definitions, and module declarations
	symbolPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^\s*(def|class|module|func|type|struct|interface|fn|pub fn|function|export function|async function)\b.*`),
		regexp.MustCompile(`(?i)^\s*(import|require|include|use|package|from)\b.*`),
	}
)

func (s *ContextSlicer) formatTreeSitterSymbols(relPath string, rawContent string) string {
	lines := strings.Split(rawContent, "\n")
	if len(lines) <= 25 {
		return formatFullFile(relPath, rawContent)
	}

	var extracted []string
	scanner := bufio.NewScanner(strings.NewReader(rawContent))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		text := scanner.Text()
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}
		for _, pat := range symbolPatterns {
			if pat.MatchString(trimmed) {
				extracted = append(extracted, fmt.Sprintf("L%d: %s", lineNo, trimmed))
				break
			}
		}
	}

	if len(extracted) == 0 {
		return formatFullFile(relPath, rawContent)
	}

	symbolSummary := strings.Join(extracted, "\n")
	return fmt.Sprintf("File %s (Tree-Sitter AST Symbol Map - %d symbols extracted):\n```\n%s\n```",
		relPath, len(extracted), symbolSummary)
}

// SliceFileFromDisk reads and slices a file from disk given the project root and relative path.
func (s *ContextSlicer) SliceFileFromDisk(projectPath string, relPath string) (string, error) {
	fullPath := relPath
	if !strings.HasPrefix(relPath, projectPath) {
		fullPath = fmt.Sprintf("%s/%s", strings.TrimSuffix(projectPath, "/"), strings.TrimPrefix(relPath, "/"))
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return s.SliceFileContext(relPath, string(content), ""), nil
}
