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
	"utilizes", "uses",
	"Utilizes", "Uses",
	"utilizing", "using",
	"Utilizing", "Using",
	"facilitate", "help",
	"Facilitate", "Help",
	"facilitates", "helps",
	"demonstrate", "show",
	"Demonstrate", "Show",
	"demonstrates", "shows",
	"commence", "start",
	"Commence", "Start",
	"commences", "starts",
	"terminate", "end",
	"Terminate", "End",
	"terminates", "ends",
	"is required to be", "must be",
	"are required to be", "must be",
	"has the capability to", "can",
	"have the capability to", "can",
	"is able to", "can",
	"are able to", "can",
	"in order to", "to",
	"In order to", "To",
	"with the exception of", "except",
	"at this point in time", "now",
	"due to the fact that", "because",
	"for the purpose of", "for",
	"in the event that", "if",
	"prior to", "before",
	"Prior to", "Before",
	"subsequent to", "after",
	"Subsequent to", "After",
	"subsequently", "then",
	"Subsequently", "Then",
	"in accordance with", "per",
	"as well as", "and",
	"in addition to", "besides",
	"it is necessary that", "must",
	"make an attempt to", "try to",
	"take into consideration", "consider",
	"with regard to", "for",
	"in light of the fact that", "because",
	"as a consequence of", "because of",
	"make sure that", "ensure",
	"at all times", "always",
	"as soon as possible", "immediately",
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

var cavemanFillerPrefixes = []string{
	"please ",
	"simply ",
	"strictly ",
	"kindly ",
	"basically ",
	"essentially ",
	"make sure to ",
	"be sure to ",
	"remember to ",
	"ensure that you ",
	"you should ",
	"you need to ",
	"you must ",
	"it is important to ",
	"it is crucial to ",
	"as mentioned above, ",
	"keep in mind that ",
}

var cavemanReplacer = strings.NewReplacer(
	"in order to", "to",
	"In order to", "To",
	"make sure to ", "",
	"Make sure to ", "",
	"be sure to ", "",
	"Be sure to ", "",
	"remember to ", "",
	"Remember to ", "",
	"ensure that you ", "",
	"Ensure that you ", "",
	"due to the fact that", "because",
	"for the purpose of", "for",
	"in the event that", "if",
	"without any exception", "",
	"take into account", "follow",
	"at all times", "",
	"as well as", "and",
	"prior to", "before",
	"subsequent to", "after",
	"subsequently", "then",
)

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

		// Telegraphic compaction: remove filler prefixes
		lower := strings.ToLower(line)
		for _, prefix := range cavemanFillerPrefixes {
			if strings.HasPrefix(lower, prefix) {
				line = line[len(prefix):]
				break
			}
		}

		// Apply telegraphic word replacements
		line = cavemanReplacer.Replace(line)

		// Collapse redundant bold wrapper on Markdown headings: e.g. "### **Heading**" -> "### Heading"
		if strings.HasPrefix(line, "#") {
			parts := strings.SplitN(line, " ", 2)
			if len(parts) == 2 {
				headingText := strings.TrimSpace(parts[1])
				if strings.HasPrefix(headingText, "**") && strings.HasSuffix(headingText, "**") && len(headingText) > 4 {
					line = parts[0] + " " + headingText[2:len(headingText)-2]
				}
			}
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

// CompactMarkdownSpecWithMode performs compaction on Markdown specifications and prompts using
// either "caveman" (telegraphic) or "simple_english" mode.
func CompactMarkdownSpecWithMode(prompt string, mode string) string {
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
	content := strings.Join(stripped, "\n")
	if strings.ToLower(strings.TrimSpace(mode)) == "simple_english" {
		return CompactSimpleEnglish(content)
	}
	return CompactCaveman(content)
}

// CompactMarkdownSpec performs caveman-style compaction on Markdown specifications and prompts,
// stripping decorative headers, HTML comments, and conversational prose while preserving technical requirements.
func CompactMarkdownSpec(prompt string) string {
	return CompactMarkdownSpecWithMode(prompt, "caveman")
}
