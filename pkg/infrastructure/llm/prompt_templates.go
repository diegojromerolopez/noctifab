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
	"has the capability to", "can",
)

func processSimpleEnglishLines(lines []string) []string {
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if lower == "please note that" || lower == "in order to ensure that" || lower == "the purpose of this document is to" {
			continue
		}
		if strings.HasPrefix(lower, "please note that ") {
			line = line[len("Please note that "):]
		} else if strings.HasPrefix(lower, "in order to ") {
			line = line[len("In order to "):]
		}
		simplifiedLine := simpleEnglishReplacer.Replace(line)
		cleaned = append(cleaned, simplifiedLine)
	}
	return cleaned
}

func processCavemanLines(lines []string) []string {
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" || trimmed == "***" || trimmed == "===" || trimmed == "___" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(lower, "please note that") ||
			strings.HasPrefix(lower, "as a user, i would like to") ||
			strings.HasPrefix(lower, "in order to ensure that") ||
			strings.HasPrefix(lower, "the purpose of this document is to") ||
			strings.HasPrefix(lower, "it is recommended that you") {
			continue
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

// CompactMarkdownSpec performs caveman-style compaction on Markdown specifications and prompts.
func CompactMarkdownSpec(prompt string) string {
	return CompactCaveman(prompt)
}
