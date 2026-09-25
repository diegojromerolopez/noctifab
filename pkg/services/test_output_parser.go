package services

import (
	"bufio"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ParsedTestReport represents normalized, language-agnostic test results parsed
// from any standard test framework protocol (JUnit XML, TAP, JSON events, or standardized text).
type ParsedTestReport struct {
	Protocol    string   `json:"protocol"`
	Total       int      `json:"total"`
	Passed      int      `json:"passed"`
	Failed      int      `json:"failed"`
	Skipped     int      `json:"skipped"`
	Success     bool     `json:"success"`
	Failures    []string `json:"failures,omitempty"`
	SummaryText string   `json:"summary_text"`
}

var (
	// Unified standard test count regexes across languages
	rePassed      = regexp.MustCompile(`(?i)\b(\d+)\s+(?:tests?\s+)?passed\b`)
	reFailed      = regexp.MustCompile(`(?i)\b(\d+)\s+(?:tests?\s+)?failed\b`)
	reErrors      = regexp.MustCompile(`(?i)\b(\d+)\s+(?:tests?\s+)?errors?\b`)
	reRanTests    = regexp.MustCompile(`(?i)\bran(?:ning)?\s+(\d+)\s+tests?\b`)
	reTestsRun    = regexp.MustCompile(`(?i)\btests?\s+run:\s*(\d+)`)
	reFailuresTag = regexp.MustCompile(`(?i)\bfailures?:\s*(\d+)`)
	reExamples    = regexp.MustCompile(`(?i)\b(\d+)\s+examples?,\s*(\d+)\s+failures?\b`)
	reGoPass      = regexp.MustCompile(`^--- PASS:\s+(\S+)`)
	reGoFail      = regexp.MustCompile(`^--- FAIL:\s+(\S+)`)
	reTAPPlan     = regexp.MustCompile(`^1\.\.(\d+)`)
	reTAPOk       = regexp.MustCompile(`^ok\s+\d*(?:\s+-\s*(.*))?`)
	reTAPNotOk    = regexp.MustCompile(`^not ok\s+\d*(?:\s+-\s*(.*))?`)
)

// ParseTestOutput analyzes command output from any programming language test runner
// and returns a normalized ParsedTestReport.
func ParseTestOutput(output string, exitCode int) ParsedTestReport {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ParsedTestReport{
			Protocol:    "empty",
			Success:     exitCode == 0,
			SummaryText: "Empty output",
		}
	}

	// 1. Try JUnit XML Parser
	if report, ok := tryParseJUnitXML(trimmed); ok {
		report.Success = report.Failed == 0 && report.Total > 0 && exitCode == 0
		return report
	}

	// 2. Try JSON Line Parser (e.g. go test -json, cargo test --format json)
	if report, ok := tryParseJSONEvents(trimmed); ok {
		report.Success = report.Failed == 0 && report.Total > 0 && exitCode == 0
		return report
	}

	// 3. Try TAP (Test Anything Protocol) Parser
	if report, ok := tryParseTAP(trimmed); ok {
		report.Success = report.Failed == 0 && report.Total > 0 && exitCode == 0
		return report
	}

	// 4. Try Unified Standard Summary Metrics (Regex)
	report := parseUnifiedMetrics(trimmed)
	if exitCode != 0 {
		report.Success = false
	} else if report.Failed > 0 || (report.Total == 0 && report.Passed == 0) {
		report.Success = false
	} else {
		report.Success = true
	}

	// 5. Anti-False-Zero Validation:
	// If exitCode was reported as 0, but a fatal crash or masked signal occurred in output, mark report as failed!
	if exitCode == 0 && report.Success {
		if violation := DetectFalseZeroExit(output, exitCode); violation.Detected {
			report.Success = false
			report.Failures = append(report.Failures, fmt.Sprintf("[Anti-False-Zero] %s (signal: %s, line: %q)", violation.Reason, violation.Signal, violation.MatchedLine))
			report.SummaryText = fmt.Sprintf("False-zero pass rejected: %s", violation.Reason)
		}
	}

	return report
}

// JUnit XML data structures
type jUnitTestSuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Suites   []jUnitTestSuite `xml:"testsuite"`
}

type jUnitTestSuite struct {
	XMLName   xml.Name        `xml:"testsuite"`
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	TestCases []jUnitTestCase `xml:"testcase"`
}

type jUnitTestCase struct {
	Name    string        `xml:"name,attr"`
	Failure *jUnitFailure `xml:"failure"`
	Error   *jUnitFailure `xml:"error"`
}

type jUnitFailure struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

func tryParseJUnitXML(raw string) (ParsedTestReport, bool) {
	if !strings.Contains(raw, "<testsuite") {
		return ParsedTestReport{}, false
	}

	// Find the XML envelope
	start := strings.Index(raw, "<testsuite")
	if start == -1 {
		return ParsedTestReport{}, false
	}
	xmlData := raw[start:]
	end := strings.LastIndex(xmlData, ">")
	if end == -1 {
		return ParsedTestReport{}, false
	}
	xmlData = xmlData[:end+1]

	var suites jUnitTestSuites
	if err := xml.Unmarshal([]byte(xmlData), &suites); err == nil && (suites.Tests > 0 || len(suites.Suites) > 0) {
		total := suites.Tests
		failed := suites.Failures + suites.Errors
		var failures []string
		for _, s := range suites.Suites {
			for _, tc := range s.TestCases {
				if tc.Failure != nil {
					failures = append(failures, fmt.Sprintf("%s: %s", tc.Name, tc.Failure.Message))
				}
				if tc.Error != nil {
					failures = append(failures, fmt.Sprintf("%s (error): %s", tc.Name, tc.Error.Message))
				}
			}
		}
		return ParsedTestReport{
			Protocol:    "junit_xml",
			Total:       total,
			Passed:      total - failed,
			Failed:      failed,
			Failures:    failures,
			SummaryText: fmt.Sprintf("JUnit XML: %d total, %d passed, %d failed", total, total-failed, failed),
		}, true
	}

	var singleSuite jUnitTestSuite
	if err := xml.Unmarshal([]byte(xmlData), &singleSuite); err == nil && (singleSuite.Tests > 0 || len(singleSuite.TestCases) > 0) {
		total := singleSuite.Tests
		if total == 0 {
			total = len(singleSuite.TestCases)
		}
		failed := singleSuite.Failures + singleSuite.Errors
		var failures []string
		for _, tc := range singleSuite.TestCases {
			if tc.Failure != nil {
				failures = append(failures, fmt.Sprintf("%s: %s", tc.Name, tc.Failure.Message))
			}
			if tc.Error != nil {
				failures = append(failures, fmt.Sprintf("%s (error): %s", tc.Name, tc.Error.Message))
			}
		}
		return ParsedTestReport{
			Protocol:    "junit_xml",
			Total:       total,
			Passed:      total - failed,
			Failed:      failed,
			Failures:    failures,
			SummaryText: fmt.Sprintf("JUnit XML: %d total, %d passed, %d failed", total, total-failed, failed),
		}, true
	}

	return ParsedTestReport{}, false
}

func tryParseJSONEvents(raw string) (ParsedTestReport, bool) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	passCount, failCount := 0, 0
	var failures []string
	jsonFound := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
			continue
		}
		var evt map[string]any
		if err := json.Unmarshal([]byte(line), &evt); err == nil {
			jsonFound = true
			action, _ := evt["Action"].(string)
			testName, _ := evt["Test"].(string)
			if action == "pass" && testName != "" {
				passCount++
			} else if action == "fail" && testName != "" {
				failCount++
				failures = append(failures, testName)
			}
		}
	}

	if jsonFound && (passCount > 0 || failCount > 0) {
		total := passCount + failCount
		return ParsedTestReport{
			Protocol:    "json_events",
			Total:       total,
			Passed:      passCount,
			Failed:      failCount,
			Failures:    failures,
			SummaryText: fmt.Sprintf("JSON Events: %d total, %d passed, %d failed", total, passCount, failCount),
		}, true
	}

	return ParsedTestReport{}, false
}

func tryParseTAP(raw string) (ParsedTestReport, bool) {
	scanner := bufio.NewScanner(strings.NewReader(raw))
	passCount, failCount := 0, 0
	var failures []string
	hasTAPHeader := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if reTAPPlan.MatchString(line) {
			hasTAPHeader = true
		} else if m := reTAPOk.FindStringSubmatch(line); len(m) > 0 {
			passCount++
		} else if m := reTAPNotOk.FindStringSubmatch(line); len(m) > 0 {
			failCount++
			desc := "unnamed test"
			if len(m) > 1 && m[1] != "" {
				desc = strings.TrimSpace(m[1])
			}
			failures = append(failures, desc)
		}
	}

	if hasTAPHeader || (passCount+failCount > 0 && strings.Contains(raw, "ok ")) {
		total := passCount + failCount
		return ParsedTestReport{
			Protocol:    "tap",
			Total:       total,
			Passed:      passCount,
			Failed:      failCount,
			Failures:    failures,
			SummaryText: fmt.Sprintf("TAP: %d total, %d passed, %d failed", total, passCount, failCount),
		}, true
	}

	return ParsedTestReport{}, false
}

func parseUnifiedMetrics(raw string) ParsedTestReport {
	passed, failed, total := 0, 0, 0
	var failures []string

	// 1. Look for RSpec style "X examples, Y failures"
	if m := reExamples.FindStringSubmatch(raw); len(m) == 3 {
		ex, _ := strconv.Atoi(m[1])
		fl, _ := strconv.Atoi(m[2])
		return ParsedTestReport{
			Protocol:    "rspec",
			Total:       ex,
			Passed:      ex - fl,
			Failed:      fl,
			SummaryText: fmt.Sprintf("RSpec: %d examples, %d failures", ex, fl),
		}
	}

	// 2. Scan lines for Go test pass/fail lines
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if m := reGoPass.FindStringSubmatch(line); len(m) > 1 {
			passed++
		} else if m := reGoFail.FindStringSubmatch(line); len(m) > 1 {
			failed++
			failures = append(failures, m[1])
		}
	}
	if passed > 0 || failed > 0 {
		return ParsedTestReport{
			Protocol:    "unified_tests",
			Total:       passed + failed,
			Passed:      passed,
			Failed:      failed,
			Failures:    failures,
			SummaryText: fmt.Sprintf("%d passed, %d failed", passed, failed),
		}
	}

	// 3. Fallback to standard token matching across Python/Rust/Jest/CTest
	if m := rePassed.FindStringSubmatch(raw); len(m) > 1 {
		passed, _ = strconv.Atoi(m[1])
	}
	if m := reFailed.FindStringSubmatch(raw); len(m) > 1 {
		failed, _ = strconv.Atoi(m[1])
	} else if m := reFailuresTag.FindStringSubmatch(raw); len(m) > 1 {
		failed, _ = strconv.Atoi(m[1])
	}
	if m := reErrors.FindStringSubmatch(raw); len(m) > 1 {
		errs, _ := strconv.Atoi(m[1])
		failed += errs
	}
	if m := reRanTests.FindStringSubmatch(raw); len(m) > 1 {
		total, _ = strconv.Atoi(m[1])
	} else if m := reTestsRun.FindStringSubmatch(raw); len(m) > 1 {
		total, _ = strconv.Atoi(m[1])
	}

	if total == 0 {
		total = passed + failed
	}

	return ParsedTestReport{
		Protocol:    "standard_metrics",
		Total:       total,
		Passed:      passed,
		Failed:      failed,
		Failures:    failures,
		SummaryText: fmt.Sprintf("%d total, %d passed, %d failed", total, passed, failed),
	}
}
