package services

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
)

var (
	// secretRegexes matches typical credential patterns (API keys, tokens, passwords).
	secretRegexes = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret|bearer)\s*[:=]\s*["']?([^\s"']+)`),
		regexp.MustCompile(`(ghp_[A-Za-z0-9_]{36}|sk-[A-Za-z0-9_]{32,}|gho_[A-Za-z0-9_]{36})`),
	}
)

// TailLogFile reads up to maxLines lines from the end of the specified log file.
// If maxLines <= 0, defaults to 50 lines.
func TailLogFile(logPath string, maxLines int) ([]string, error) {
	if maxLines <= 0 {
		maxLines = 50
	}

	file, err := os.Open(logPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	// Read lines efficiently using bufio.Scanner
	scanner := bufio.NewScanner(file)
	// Allow scanning long lines (up to 1MB per line)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	var ring []string
	for scanner.Scan() {
		line := scanner.Text()
		ring = append(ring, line)
		if len(ring) > maxLines {
			ring = ring[1:]
		}
	}

	if err := scanner.Err(); err != nil && err != io.EOF {
		return ring, err
	}

	return ring, nil
}

// SanitizeLog redacts secrets (tokens, API keys, passwords) from a log string or snippet.
func SanitizeLog(log string) string {
	sanitized := log
	for _, re := range secretRegexes {
		sanitized = re.ReplaceAllStringFunc(sanitized, func(match string) string {
			parts := re.FindStringSubmatch(match)
			if len(parts) >= 3 {
				// Replace secret value part with REDACTED
				return strings.Replace(match, parts[2], "[REDACTED_SECRET]", 1)
			}
			// General match replace
			return "[REDACTED_SECRET]"
		})
	}
	return sanitized
}
