package services

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Assertion patterns that check for HTTP 200 / success status
	status200RE = regexp.MustCompile(`(?i)(?:status_code\s*==\s*200|status\s*==\s*200|assert_response_status\s*\(.*?200\)|assert\s+r\.status\s*==\s*200|assert\s+resp\.status_code\s*==\s*200|assert_equal\s*\(.*?200\)|code\s*==\s*200|assert\s+status\s*==\s*200)`)

	// Assertion patterns that check for negative / error statuses (404, 400, 405, 500, error)
	negativeStatusRE = regexp.MustCompile(`(?i)(?:status_code\s*==\s*(?:400|404|405|500)|status\s*==\s*(?:400|404|405|500)|assert.*?(?:404|400|405|500)|assert.*?(?:not\s+found|error|invalid|err)|assert_raises|assertRaises|assert_response_status\s*\(.*?(?:404|400)\))`)

	// HTTP request in test pattern
	testHTTPRequestRE = regexp.MustCompile(`(?i)(?:requests\.(?:get|post|put|delete)|http_client|curl\s+|socket\.connect|conn\.request|http\.get|http\.post)`)
)

// checkTestSensitivityViolations inspects test files to ensure that integration/E2E
// endpoint tests include negative assertions rather than solely testing 200 OK.
func checkTestSensitivityViolations(path string, lines []string) []AntiStubViolation {
	if !isTestFilePath(path) {
		return nil
	}

	var violations []AntiStubViolation
	hasHTTPRequest := false
	status200Count := 0
	negativeCount := 0
	first200Line := 0

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if testHTTPRequestRE.MatchString(trimmed) {
			hasHTTPRequest = true
		}

		if status200RE.MatchString(trimmed) {
			status200Count++
			if first200Line == 0 {
				first200Line = idx + 1
			}
		}

		if negativeStatusRE.MatchString(trimmed) {
			negativeCount++
		}
	}

	// If the test suite performs HTTP requests and makes multiple 200 assertions,
	// but contains zero negative status assertions (e.g. 404, 400), flag it.
	if hasHTTPRequest && status200Count >= 3 && negativeCount == 0 {
		violations = append(violations, AntiStubViolation{
			Path: path,
			Line: first200Line,
			Rule: "lacks_negative_assertion_coverage",
			Snippet: fmt.Sprintf(
				"Test file contains %d positive status assertions (200 OK) but 0 negative or error assertions (e.g. 404 Not Found, 400 Bad Request)",
				status200Count,
			),
		})
	}

	return violations
}
