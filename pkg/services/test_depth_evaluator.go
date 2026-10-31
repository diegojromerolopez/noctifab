package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// DepthEvaluationResult reports whether a test suite satisfies the anti-shallowness criteria.
type DepthEvaluationResult struct {
	Passed             bool     `json:"passed"`
	IsShallow          bool     `json:"is_shallow"`
	MissingDimensions  []string `json:"missing_dimensions"`
	RemediationMessage string   `json:"remediation_message,omitempty"`
}

// TestDepthEvaluator assesses test sources for exhaustiveness, dual-layer state verification, and boundary checks.
type TestDepthEvaluator struct{}

// NewTestDepthEvaluator constructs a new TestDepthEvaluator.
func NewTestDepthEvaluator() *TestDepthEvaluator {
	return &TestDepthEvaluator{}
}

var (
	reStatusOnlyCheck = regexp.MustCompile(`(?i)(assert.*(?:status|code|startswith|contains).*(?:200|201|204|206|400|404))`)
	reBodyCheck       = regexp.MustCompile(`(?i)(body\s*==|endswith\(body|assert.*(?:body|content|payload)|json\.loads|deserialize|expected_body|strlen\(body\)|strcmp\(.*body)`)
	reHeaderCheck     = regexp.MustCompile(`(?i)(assert.*(?:etag|server|content-type|content-length|content-range|headers\[))`)
	reStateCheck      = regexp.MustCompile(`(?i)(os\.path\.exists|os\.listdir|open\(|pathlib|readdir|stat\(|access\(|SELECT.*FROM|sqlite3|test_store_exists|t4_store_get|read_file)`)
	reNegativeCheck   = regexp.MustCompile(`(?i)(400|404|405|413|416|invalid|missing|unknown|not_found|bad_request|traversal|\.\./|%2e%2e)`)
	reBoundaryCheck   = regexp.MustCompile(`(?i)(0-0|bytes=0-|bytes=-|1048576|1048577|empty|len.*==\s*0|Content-Length:\s*0|port.*(?:0|65536))`)
)

// EvaluateDepth scans test files in target paths or the tests directory for test quality.
func (e *TestDepthEvaluator) EvaluateDepth(ctx context.Context, projectPath string, targetTestFiles []string) (*DepthEvaluationResult, error) {
	filesToScan := targetTestFiles
	if len(filesToScan) == 0 {
		// Discover test files in tests/
		testsDir := filepath.Join(projectPath, "tests")
		if _, err := os.Stat(testsDir); err == nil {
			_ = filepath.Walk(testsDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || info == nil || info.IsDir() {
					return nil
				}
				ext := filepath.Ext(path)
				base := filepath.Base(path)
				if ext == ".py" || ext == ".sh" || ext == ".c" || strings.HasSuffix(base, "_test.go") {
					filesToScan = append(filesToScan, path)
				}
				return nil
			})
		}
	}

	if len(filesToScan) == 0 {
		return &DepthEvaluationResult{
			Passed:             false,
			IsShallow:          true,
			MissingDimensions:  []string{"no test files found to evaluate"},
			RemediationMessage: "No test files were found. Tests must be authored with response body assertions and negative/boundary checks.",
		}, nil
	}

	var combinedContent strings.Builder
	existingFiles := 0
	for _, f := range filesToScan {
		fullPath := f
		if !filepath.IsAbs(fullPath) {
			fullPath = filepath.Join(projectPath, fullPath)
		}
		data, err := os.ReadFile(fullPath)
		if err == nil {
			existingFiles++
			combinedContent.WriteString(string(data))
			combinedContent.WriteString("\n")
		}
	}

	if existingFiles == 0 {
		return &DepthEvaluationResult{
			Passed:    true,
			IsShallow: false,
		}, nil
	}

	content := combinedContent.String()
	if strings.TrimSpace(content) == "" {
		return &DepthEvaluationResult{
			Passed:             false,
			IsShallow:          true,
			MissingDimensions:  []string{"test files are empty"},
			RemediationMessage: "Test files are empty stubs. Implement non-shallow assertions covering body, headers, and state.",
		}, nil
	}

	var missing []string

	hasStatus := reStatusOnlyCheck.MatchString(content)
	hasBody := reBodyCheck.MatchString(content)
	hasHeaders := reHeaderCheck.MatchString(content)
	hasState := reStateCheck.MatchString(content)
	hasNegative := reNegativeCheck.MatchString(content)
	hasBoundary := reBoundaryCheck.MatchString(content)

	isIntegrationOrE2E := false
	for _, f := range filesToScan {
		if strings.Contains(f, "e2e") || strings.Contains(f, "acceptance") || strings.Contains(f, "integration") {
			isIntegrationOrE2E = true
			break
		}
	}

	if hasStatus || isIntegrationOrE2E {
		if !hasBody && hasStatus {
			missing = append(missing, "missing response body assertions (tests check status code without asserting returned body)")
		}
		if !hasHeaders && hasStatus {
			missing = append(missing, "missing mandatory response header assertions (e.g. Server, Content-Length, ETag, Content-Type)")
		}
		if !hasState && hasStatus {
			missing = append(missing, "missing dual-layer state persistence checks (tests assert communication response but do not verify filesystem/storage state)")
		}
		if !hasNegative {
			missing = append(missing, "missing negative test scenarios (testing invalid keys, path traversal, malformed range, or 404/400 errors)")
		}
		if !hasBoundary {
			missing = append(missing, "missing boundary test scenarios (testing empty body, 0 bytes, limit payloads, or range boundaries)")
		}
	}

	if len(missing) > 0 {
		remediation := fmt.Sprintf(
			"Test suite is shallow. The following test dimensions are required before considering tests successful:\n - %s\n"+
				"Deepen the test suite: assert exact response bodies and headers, inspect underlying filesystem/state persistence, and include negative and boundary cases.",
			strings.Join(missing, "\n - "),
		)
		return &DepthEvaluationResult{
			Passed:             false,
			IsShallow:          true,
			MissingDimensions:  missing,
			RemediationMessage: remediation,
		}, nil
	}

	return &DepthEvaluationResult{
		Passed:    true,
		IsShallow: false,
	}, nil
}
