package llm

import (
	"strings"
	"sync"
)

const parallelCompactionThreshold = 20000

type promptBlock struct {
	isCode bool
	lines  []string
}

func parsePromptBlocks(lines []string) []promptBlock {
	var blocks []promptBlock
	var current []string
	inCode := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if len(current) > 0 {
				blocks = append(blocks, promptBlock{isCode: inCode, lines: current})
				current = nil
			}
			inCode = !inCode
			blocks = append(blocks, promptBlock{isCode: true, lines: []string{line}})
			continue
		}
		current = append(current, line)
	}
	if len(current) > 0 {
		blocks = append(blocks, promptBlock{isCode: inCode, lines: current})
	}
	return blocks
}

func parallelCompact(prompt string, processLines func(lines []string) []string) string {
	lines := strings.Split(prompt, "\n")
	if len(prompt) < parallelCompactionThreshold {
		var cleaned []string
		inCodeBlock := false
		var currentText []string

		flushText := func() {
			if len(currentText) > 0 {
				cleaned = append(cleaned, processLines(currentText)...)
				currentText = nil
			}
		}

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				flushText()
				inCodeBlock = !inCodeBlock
				cleaned = append(cleaned, line)
				continue
			}
			if inCodeBlock {
				cleaned = append(cleaned, line)
				continue
			}
			currentText = append(currentText, line)
		}
		flushText()
		return strings.Join(cleaned, "\n")
	}

	blocks := parsePromptBlocks(lines)
	results := make([][]string, len(blocks))

	var wg sync.WaitGroup
	for i, b := range blocks {
		if b.isCode {
			results[i] = b.lines
			continue
		}
		wg.Add(1)
		go func(idx int, blk promptBlock) {
			defer wg.Done()
			results[idx] = processLines(blk.lines)
		}(i, b)
	}
	wg.Wait()

	var finalLines []string
	for _, res := range results {
		finalLines = append(finalLines, res...)
	}
	return strings.Join(finalLines, "\n")
}

var simpleEnglishReplacer = strings.NewReplacer(
	"utilize", "use",
	"Utilize", "Use",
	"facilitate", "help",
	"Facilitate", "Help",
	"demonstrate", "show",
	"Demonstrate", "Show",
	"commence", "start",
	"Commence", "Start",
	"terminate", "end",
	"Terminate", "End",
	"is required to be", "must be",
	"are required to be", "must be",
	"has the capability to", "can",
	"have the capability to", "can",
	"in order to", "to",
	"In order to", "To",
	"with the exception of", "except",
	"at this point in time", "now",
	"due to the fact that", "because",
	"for the purpose of", "for",
	"in the event that", "if",
	"prioritize a clean, functional implementation that makes all tests pass", "make all tests pass",
)

var fullLinePreambles = []string{
	"you are a software factory automation agent operating in a restricted workspace sandbox",
	"you are acting as the generator agent",
	"you are acting as the tester agent",
	"your task is to implement the specified task",
	"focus on creating the minimal implementation/functionality to fulfill the task requirements",
}

func isConversationalPreamble(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	lower = strings.TrimSuffix(lower, ".")
	lower = strings.TrimSuffix(lower, ":")
	for _, p := range fullLinePreambles {
		if lower == p {
			return true
		}
	}
	return false
}

var stripPrefixes = []string{
	"please note that ",
	"in order to ensure that ",
	"in order to ",
	"the purpose of this document is to ",
	"it is recommended that you ",
	"as a user, i would like to ",
}

func stripPreamblePrefix(line string) string {
	lower := strings.ToLower(line)
	for _, p := range stripPrefixes {
		if strings.HasPrefix(lower, p) {
			return line[len(p):]
		}
	}
	return line
}

func processSimpleEnglishLines(lines []string) []string {
	var cleaned []string
	lastBlank := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
			cleaned = append(cleaned, "")
			continue
		}
		lastBlank = false

		if isConversationalPreamble(trimmed) {
			continue
		}

		line = stripPreamblePrefix(line)
		simplifiedLine := simpleEnglishReplacer.Replace(line)
		cleaned = append(cleaned, simplifiedLine)
	}
	return cleaned
}

func processCavemanLines(lines []string) []string {
	var cleaned []string
	lastBlank := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if lastBlank {
				continue
			}
			lastBlank = true
			cleaned = append(cleaned, "")
			continue
		}
		lastBlank = false

		if trimmed == "---" || trimmed == "***" || trimmed == "===" || trimmed == "___" {
			continue
		}

		if isConversationalPreamble(trimmed) {
			continue
		}

		line = stripPreamblePrefix(line)

		// Telegraphic compaction: remove polite filler prefixes
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "please "):
			line = line[len("Please "):]
		case strings.HasPrefix(lower, "simply "):
			line = line[len("Simply "):]
		case strings.HasPrefix(lower, "strictly "):
			line = line[len("Strictly "):]
		}

		cleaned = append(cleaned, line)
	}
	return cleaned
}

// CompactSimpleEnglish compacts prompts using Simple English rules (active voice, simple vocabulary, no conversational fluff)
// while strictly preserving code blocks, JSON schemas, filepaths, CLI flags, and technical invariants.
func CompactSimpleEnglish(prompt string) string {
	return parallelCompact(prompt, processSimpleEnglishLines)
}

// CompactCaveman performs telegraphic caveman-style compaction on prompts.
// It removes conversational fluff, polite phrases, and decorative dividers
// while strictly preserving exact filepaths, code blocks, JSON schemas, CLI flags, and technical invariants.
func CompactCaveman(prompt string) string {
	return parallelCompact(prompt, processCavemanLines)
}

// CompactMarkdownSpec performs caveman-style compaction on Markdown specifications and prompts,
// stripping decorative headers, HTML comments, and conversational prose while preserving technical requirements.
func CompactMarkdownSpec(prompt string) string {
	// Strip HTML comments (e.g. <!-- ... -->)
	lines := strings.Split(prompt, "\n")
	var stripped []string
	inComment := false
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "<!--") && strings.HasSuffix(trimmed, "-->") {
			continue
		}
		if strings.HasPrefix(trimmed, "<!--") {
			inComment = true
			continue
		}
		if inComment {
			if strings.Contains(trimmed, "-->") {
				inComment = false
			}
			continue
		}
		// Strip Markdown image links
		if strings.HasPrefix(trimmed, "![") && strings.Contains(trimmed, "](") {
			continue
		}
		stripped = append(stripped, l)
	}
	return CompactCaveman(strings.Join(stripped, "\n"))
}
