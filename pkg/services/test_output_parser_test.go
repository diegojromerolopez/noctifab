package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseTestOutput(t *testing.T) {
	t.Run("JUnit XML success", func(t *testing.T) {
		xmlOutput := `
<testsuites tests="3" failures="0" errors="0">
  <testsuite name="math" tests="3" failures="0">
    <testcase name="test_add" />
    <testcase name="test_sub" />
    <testcase name="test_mul" />
  </testsuite>
</testsuites>`
		report := ParseTestOutput(xmlOutput, 0)
		assert.Equal(t, "junit_xml", report.Protocol)
		assert.Equal(t, 3, report.Total)
		assert.Equal(t, 3, report.Passed)
		assert.Equal(t, 0, report.Failed)
		assert.True(t, report.Success)
	})

	t.Run("JUnit XML failure", func(t *testing.T) {
		xmlOutput := `
<testsuite name="api" tests="2" failures="1" errors="0">
  <testcase name="test_ok" />
  <testcase name="test_fail">
    <failure message="expected 200 got 500" />
  </testcase>
</testsuite>`
		report := ParseTestOutput(xmlOutput, 1)
		assert.Equal(t, "junit_xml", report.Protocol)
		assert.Equal(t, 2, report.Total)
		assert.Equal(t, 1, report.Failed)
		assert.False(t, report.Success)
		assert.Contains(t, report.Failures[0], "expected 200 got 500")
	})

	t.Run("JSON Line Events (go test -json)", func(t *testing.T) {
		jsonOutput := `{"Action":"run","Test":"TestAdd"}
{"Action":"pass","Test":"TestAdd"}
{"Action":"run","Test":"TestSub"}
{"Action":"pass","Test":"TestSub"}
{"Action":"run","Test":"TestFail"}
{"Action":"fail","Test":"TestFail"}`
		report := ParseTestOutput(jsonOutput, 1)
		assert.Equal(t, "json_events", report.Protocol)
		assert.Equal(t, 3, report.Total)
		assert.Equal(t, 2, report.Passed)
		assert.Equal(t, 1, report.Failed)
		assert.False(t, report.Success)
		assert.Equal(t, []string{"TestFail"}, report.Failures)
	})

	t.Run("TAP protocol", func(t *testing.T) {
		tapOutput := `1..3
ok 1 - auth service connects
not ok 2 - token validation failed
ok 3 - logout cleans cookie`
		report := ParseTestOutput(tapOutput, 1)
		assert.Equal(t, "tap", report.Protocol)
		assert.Equal(t, 3, report.Total)
		assert.Equal(t, 2, report.Passed)
		assert.Equal(t, 1, report.Failed)
		assert.False(t, report.Success)
		assert.Contains(t, report.Failures[0], "token validation failed")
	})

	t.Run("RSpec output", func(t *testing.T) {
		rspecOutput := `Finished in 0.05 seconds (files took 0.1 seconds to load)
12 examples, 0 failures`
		report := ParseTestOutput(rspecOutput, 0)
		assert.Equal(t, "rspec", report.Protocol)
		assert.Equal(t, 12, report.Total)
		assert.Equal(t, 12, report.Passed)
		assert.Equal(t, 0, report.Failed)
		assert.True(t, report.Success)
	})

	t.Run("Go test text output", func(t *testing.T) {
		goOutput := `=== RUN   TestA
--- PASS: TestA (0.01s)
=== RUN   TestB
--- FAIL: TestB (0.02s)
FAIL`
		report := ParseTestOutput(goOutput, 1)
		assert.Equal(t, "unified_tests", report.Protocol)
		assert.Equal(t, 2, report.Total)
		assert.Equal(t, 1, report.Passed)
		assert.Equal(t, 1, report.Failed)
		assert.False(t, report.Success)
		assert.Equal(t, []string{"TestB"}, report.Failures)
	})

	t.Run("Pytest output", func(t *testing.T) {
		pyOutput := `========================= 45 passed in 1.23s =========================`
		report := ParseTestOutput(pyOutput, 0)
		assert.Equal(t, 45, report.Passed)
		assert.Equal(t, 0, report.Failed)
		assert.True(t, report.Success)
	})

	t.Run("Rust cargo test output", func(t *testing.T) {
		cargoOutput := `running 8 tests
test result: ok. 8 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out`
		report := ParseTestOutput(cargoOutput, 0)
		assert.Equal(t, 8, report.Passed)
		assert.Equal(t, 0, report.Failed)
		assert.True(t, report.Success)
	})

	t.Run("Zero test run fails even with exit 0", func(t *testing.T) {
		zeroOutput := `test result: ok. 0 passed; 0 failed; 0 ignored; 0 measured; 0 filtered out`
		report := ParseTestOutput(zeroOutput, 0)
		assert.False(t, report.Success, "zero test assertions executed must fail")
	})

	t.Run("Non-zero exit code fails even if pass text present", func(t *testing.T) {
		spoofOutput := `All 10 tests passed! But process died with SIGSEGV`
		report := ParseTestOutput(spoofOutput, 139)
		assert.False(t, report.Success, "process crash must fail regardless of stdout")
	})
}
