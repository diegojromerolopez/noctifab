package services

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Accept call patterns
	cAcceptRE  = regexp.MustCompile(`\b(?:int\s+)?([a-zA-Z0-9_]+)\s*=\s*accept\s*\(`)
	pyAcceptRE = regexp.MustCompile(`([a-zA-Z0-9_]+)\s*(?:,\s*[a-zA-Z0-9_]+)?\s*=\s*(?:[a-zA-Z0-9_]+\.)?accept\s*\(`)
	goAcceptRE = regexp.MustCompile(`([a-zA-Z0-9_]+)\s*(?:,\s*[a-zA-Z0-9_]+)?\s*:?=\s*(?:[a-zA-Z0-9_]+\.)?Accept\s*\(`)

	// Hardcoded protocol responses
	hardcodedProtocolLiteralRE = regexp.MustCompile(`(?i)["'](?:HTTP/1\.[01]\s+200|\+PONG|\+OK|HTTP/1\.[01]\s+[0-9]{3}|{\s*"status"\s*:\s*"ok"\s*})`)

	// Read / Recv patterns
	socketReadRE = regexp.MustCompile(`\b(?:recv|recvfrom|read|readv|getline|fscanf|conn\.recv|conn\.read|conn\.Read|bufio\.NewReader)\b`)
)

// checkDaemonSocketViolations inspects production daemon and server source files
// for sockets that immediately reply with hardcoded protocol literals without reading client requests.
func checkDaemonSocketViolations(path string, lines []string) []AntiStubViolation {
	if isTestFilePath(path) {
		return nil
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".c" && ext != ".cpp" && ext != ".cc" && ext != ".go" && ext != ".py" && ext != ".rs" {
		return nil
	}

	var violations []AntiStubViolation
	numLines := len(lines)

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}

		var connVar string
		if match := cAcceptRE.FindStringSubmatch(trimmed); len(match) > 1 {
			connVar = match[1]
		} else if match := pyAcceptRE.FindStringSubmatch(trimmed); len(match) > 1 {
			connVar = match[1]
		} else if match := goAcceptRE.FindStringSubmatch(trimmed); len(match) > 1 {
			connVar = match[1]
		}

		if connVar == "" {
			continue
		}

		// Look ahead up to 35 lines or until block close
		hasRead := false
		for nextIdx := idx + 1; nextIdx < numLines && nextIdx < idx+35; nextIdx++ {
			nextLine := strings.TrimSpace(lines[nextIdx])
			if nextLine == "" || strings.HasPrefix(nextLine, "//") || strings.HasPrefix(nextLine, "#") {
				continue
			}

			if socketReadRE.MatchString(nextLine) {
				hasRead = true
				break
			}

			// Check if it writes a hardcoded protocol response to connVar
			if hardcodedProtocolLiteralRE.MatchString(nextLine) {
				isWrite := strings.Contains(nextLine, "send") ||
					strings.Contains(nextLine, "write") ||
					strings.Contains(nextLine, "dprintf") ||
					strings.Contains(nextLine, "Write")
				if isWrite && !hasRead {
					violations = append(violations, AntiStubViolation{
						Path:    path,
						Line:    nextIdx + 1,
						Rule:    "hardcoded_unparsing_server_stub",
						Snippet: nextLine,
					})
					break
				}
			}
		}
	}

	return violations
}
