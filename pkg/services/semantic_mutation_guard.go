package services

import (
	"fmt"
	"regexp"
	"strings"
)

// SemanticMutationViolation reports a change that is superficial (e.g. comments, whitespace,
// or standalone debug print statements) with no actual business logic modification.
type SemanticMutationViolation struct {
	FilePath string
	Reason   string
}

func (v SemanticMutationViolation) Error() string {
	return fmt.Sprintf("semantic mutation violation in %s: %s; no-op repair loops are prohibited",
		v.FilePath, v.Reason)
}

// SemanticMutationGuard evaluates whether modifications made to a file contain
// meaningful structural code changes rather than cosmetic no-op edits.
type SemanticMutationGuard struct {
	debugPrintPatterns []*regexp.Regexp
}

// NewSemanticMutationGuard creates a new SemanticMutationGuard.
func NewSemanticMutationGuard() *SemanticMutationGuard {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`^\s*print\s*\(.*\)\s*;?$`),
		regexp.MustCompile(`^\s*println\s*\(.*\)\s*;?$`),
		regexp.MustCompile(`^\s*console\.(log|debug|info|warn|error)\s*\(.*\)\s*;?$`),
		regexp.MustCompile(`^\s*fmt\.Print(ln|f)?\s*\(.*\)\s*;?$`),
		regexp.MustCompile(`^\s*log\.(Print|Debug|Info)(ln|f)?\s*\(.*\)\s*;?$`),
		regexp.MustCompile(`^\s*logger\.(info|debug|warn|error)\s*\(.*\)\s*;?$`),
		regexp.MustCompile(`^\s*eprintln!\s*\(.*\)\s*;?$`),
		regexp.MustCompile(`^\s*println!\s*\(.*\)\s*;?$`),
	}
	return &SemanticMutationGuard{
		debugPrintPatterns: patterns,
	}
}

// ValidateSemanticChange compares original and updated code content.
// Returns an error if the change is empty or only consists of comments, whitespace,
// or debug logging statements.
func (g *SemanticMutationGuard) ValidateSemanticChange(filePath, originalContent, newContent string) error {
	if originalContent == newContent {
		return &SemanticMutationViolation{
			FilePath: filePath,
			Reason:   "file content is identical (zero byte mutation)",
		}
	}

	normOrig := g.normalizeCode(originalContent)
	normNew := g.normalizeCode(newContent)

	if normOrig == normNew {
		return &SemanticMutationViolation{
			FilePath: filePath,
			Reason:   "change only modifies comments, docstrings, or whitespace with zero logic alterations",
		}
	}

	// Check if the only difference between normOrig and normNew is standalone debug prints
	if g.isOnlyDebugPrintsAdded(normOrig, normNew) {
		return &SemanticMutationViolation{
			FilePath: filePath,
			Reason:   "change only adds or alters debug print/logging statements without functional logic modification",
		}
	}

	return nil
}

// normalizeCode removes comments, docstrings, and empty lines, and normalizes intra-line whitespace.
func (g *SemanticMutationGuard) normalizeCode(code string) string {
	lines := strings.Split(code, "\n")
	var meaningful []string

	inBlockComment := false
	inDocstringTripleQuote := false
	inDocstringTripleDoubleQuote := false

	for _, rawLine := range lines {
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			continue
		}

		// Handle block comments /* ... */
		if inBlockComment {
			if strings.Contains(trimmed, "*/") {
				parts := strings.SplitN(trimmed, "*/", 2)
				inBlockComment = false
				trimmed = strings.TrimSpace(parts[1])
			} else {
				continue
			}
		}

		// Handle Python triple single-quotes
		if inDocstringTripleQuote {
			if strings.Contains(trimmed, "'''") {
				parts := strings.SplitN(trimmed, "'''", 2)
				inDocstringTripleQuote = false
				trimmed = strings.TrimSpace(parts[1])
			} else {
				continue
			}
		}

		// Handle Python triple double-quotes
		if inDocstringTripleDoubleQuote {
			if strings.Contains(trimmed, `"""`) {
				parts := strings.SplitN(trimmed, `"""`, 2)
				inDocstringTripleDoubleQuote = false
				trimmed = strings.TrimSpace(parts[1])
			} else {
				continue
			}
		}

		if strings.HasPrefix(trimmed, "/*") {
			if strings.Contains(trimmed, "*/") {
				parts := strings.SplitN(trimmed, "*/", 2)
				trimmed = strings.TrimSpace(parts[1])
			} else {
				inBlockComment = true
				continue
			}
		}

		if strings.HasPrefix(trimmed, `"""`) {
			if strings.Count(trimmed, `"""`) >= 2 {
				continue
			}
			inDocstringTripleDoubleQuote = true
			continue
		}

		if strings.HasPrefix(trimmed, `'''`) {
			if strings.Count(trimmed, `'''`) >= 2 {
				continue
			}
			inDocstringTripleQuote = true
			continue
		}

		// Line comments
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "--") {
			continue
		}

		if trimmed != "" {
			// Collapse internal whitespace
			words := strings.Fields(trimmed)
			meaningful = append(meaningful, strings.Join(words, " "))
		}
	}

	return strings.Join(meaningful, "\n")
}

// isOnlyDebugPrintsAdded checks if lines present in newCode but missing in origCode are solely debug prints.
func (g *SemanticMutationGuard) isOnlyDebugPrintsAdded(origNorm, newNorm string) bool {
	origLines := strings.Split(origNorm, "\n")
	newLines := strings.Split(newNorm, "\n")

	origSet := make(map[string]int)
	for _, l := range origLines {
		origSet[l]++
	}

	addedCount := 0
	debugPrintCount := 0

	for _, l := range newLines {
		if origSet[l] > 0 {
			origSet[l]--
			continue
		}

		addedCount++
		if g.matchesDebugPrint(l) {
			debugPrintCount++
		}
	}

	return addedCount > 0 && addedCount == debugPrintCount
}

func (g *SemanticMutationGuard) matchesDebugPrint(line string) bool {
	for _, pat := range g.debugPrintPatterns {
		if pat.MatchString(line) {
			return true
		}
	}
	return false
}
