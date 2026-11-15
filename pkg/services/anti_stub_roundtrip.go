package services

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	// Detection of write / mutate operations
	stateWriteRE = regexp.MustCompile(`(?i)\b(?:set|hset|lpush|rpush|sadd|zadd|put|insert|post|write|save|store)\s*\(`)

	// Detection of read / fetch operations
	stateReadRE = regexp.MustCompile(`(?i)\b(?:get|hget|lpop|rpop|smembers|zrange|fetch|read|select|query)\s*\(`)

	// Detection of dynamic / random value generation
	dynamicValueRE = regexp.MustCompile(`(?i)\b(?:uuid|uuid4|uuid_generate|rand|random|secrets|token_hex|token_bytes|urandom|time\.time|time\.Now|timestamp|UnixNano|mktemp|getpid|SystemTime|Math\.random|randomUUID)\b`)

	// Static dummy key-value literals commonly used in tautological stubs
	staticDummyPairRE = regexp.MustCompile(`(?i)\(\s*["'](?:foo|bar|key|k1|test|val|v1)["']\s*,\s*["'](?:foo|bar|val|v1|value|123|hello)["']\s*\)`)
)

// checkDynamicRoundTripViolations inspects test files to ensure that stateful CRUD/store
// tests do not solely rely on hardcoded static string constants without randomized/dynamic value verification.
func checkDynamicRoundTripViolations(path string, lines []string) []AntiStubViolation {
	if !isTestFilePath(path) {
		return nil
	}

	var violations []AntiStubViolation
	inTestFunc := false
	testFuncStart := 0
	testFuncName := ""
	hasWrite := false
	hasRead := false
	hasDynamic := false
	hasStaticDummy := false
	var dummySnippet string

	flushFunc := func() {
		if inTestFunc && hasWrite && hasRead && hasStaticDummy && !hasDynamic {
			violations = append(violations, AntiStubViolation{
				Path: path,
				Line: testFuncStart,
				Rule: "e2e_hardcoded_static_test",
				Snippet: fmt.Sprintf(
					"Test %s performs stateful write/read round-trip with static literal %s without dynamic/random value verification",
					testFuncName, dummySnippet,
				),
			})
		}
		inTestFunc = false
		hasWrite = false
		hasRead = false
		hasDynamic = false
		hasStaticDummy = false
		dummySnippet = ""
	}

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Check for test function start
		if strings.HasPrefix(trimmed, "def test_") ||
			strings.HasPrefix(trimmed, "func Test") ||
			strings.HasPrefix(trimmed, "void test_") ||
			strings.HasPrefix(trimmed, "int test_") ||
			strings.HasPrefix(trimmed, "fn test_") ||
			strings.HasPrefix(trimmed, "it(") ||
			strings.HasPrefix(trimmed, "test(") {
			flushFunc()
			inTestFunc = true
			testFuncStart = idx + 1
			testFuncName = strings.Fields(trimmed)[1]
			continue
		}

		if inTestFunc {
			if stateWriteRE.MatchString(trimmed) {
				hasWrite = true
			}
			if stateReadRE.MatchString(trimmed) {
				hasRead = true
			}
			if dynamicValueRE.MatchString(trimmed) {
				hasDynamic = true
			}
			if match := staticDummyPairRE.FindString(trimmed); match != "" {
				hasStaticDummy = true
				if dummySnippet == "" {
					dummySnippet = match
				}
			}
		}
	}
	flushFunc()

	return violations
}
