package services

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// OscillationAction represents the circuit breaker verdict.
type OscillationAction string

const (
	// OscillationActionContinue indicates no cycle was detected; continue standard turns.
	OscillationActionContinue OscillationAction = "CONTINUE"
	// OscillationActionReconcile indicates an oscillation cycle was detected for the first time.
	// Inject a targeted reconciliation directive instructing the agent to reconcile or delete conflicting tests.
	OscillationActionReconcile OscillationAction = "RECONCILE"
	// OscillationActionTrip indicates a recurring or persistent cycle after reconciliation; halt loop immediately.
	OscillationActionTrip OscillationAction = "TRIP"
)

// OscillationDecision contains the verdict of the circuit breaker for a given turn.
type OscillationDecision struct {
	Action    OscillationAction
	Period    int
	Reason    string
	Directive string
	StateA    string
	StateB    string
}

// FailureState records a single turn's normalized failure state.
type FailureState struct {
	Turn    int
	Hash    string
	Summary string
}

var (
	// Volatile normalization regexes
	memAddrRegex  = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	durationRegex = regexp.MustCompile(`\b\d+\.\d+s\b|\b\d+ms\b`)
	lineNumRegex  = regexp.MustCompile(`(?:line\s+|:)\d+(?::\d+)?`)
	turnNumRegex  = regexp.MustCompile(`(?:Turn\s+|turn\s+)\d+(?:/\d+)?`)
	strikeRegex   = regexp.MustCompile(`(?:strike\s+|Strike\s+)\d+/\d+`)

	// Structured error pattern extractors across ecosystems
	pythonTestFailRegex = regexp.MustCompile(`(?m)^(?:FAIL|ERROR):\s+([a-zA-Z0-9_\.]+)\s+\((.+)\)`)
	goTestFailRegex     = regexp.MustCompile(`(?m)^---\s+FAIL:\s+([a-zA-Z0-9_]+)`)
	cargoTestFailRegex  = regexp.MustCompile(`(?m)^test\s+([a-zA-Z0-9_:]+)\s+\.\.\.\s+FAILED`)
	jestTestFailRegex   = regexp.MustCompile(`(?m)^\s*(?:✕|FAIL)\s+([^\n\r]+)`)
	errorClassRegex     = regexp.MustCompile(`(?m)^([a-zA-Z0-9_]+Error|panic|undefined:|[a-zA-Z0-9_]+Exception):\s*(.+)`)
)

// OscillationCircuitBreaker detects periodic and ping-pong failure state oscillations
// across autonomous repair loops and intervenes before tokens are wasted.
type OscillationCircuitBreaker struct {
	history              []FailureState
	maxWindow            int
	reconciliationsCount int
	maxReconciliations   int
	maxCyclePeriod       int
}

// NewOscillationCircuitBreaker creates a circuit breaker with sensible defaults.
func NewOscillationCircuitBreaker() *OscillationCircuitBreaker {
	return &OscillationCircuitBreaker{
		maxWindow:          12,
		maxReconciliations: 1, // Intervene with reconciliation once; trip if cycle persists
		maxCyclePeriod:     4, // Detect cycles of period 2 (A-B-A), 3 (A-B-C-A), or 4
	}
}

// Fingerprint extracts a deterministic normalized signature and summary from failure logs.
func (cb *OscillationCircuitBreaker) Fingerprint(logContent string) (hash string, summary string) {
	if strings.TrimSpace(logContent) == "" {
		return "empty_pass", "No errors reported"
	}

	var components []string

	// 1. Extract test failure identifiers
	for _, m := range pythonTestFailRegex.FindAllStringSubmatch(logContent, -1) {
		if len(m) > 1 {
			components = append(components, "py_fail:"+m[1])
		}
	}
	for _, m := range goTestFailRegex.FindAllStringSubmatch(logContent, -1) {
		if len(m) > 1 {
			components = append(components, "go_fail:"+m[1])
		}
	}
	for _, m := range cargoTestFailRegex.FindAllStringSubmatch(logContent, -1) {
		if len(m) > 1 {
			components = append(components, "cargo_fail:"+m[1])
		}
	}
	for _, m := range jestTestFailRegex.FindAllStringSubmatch(logContent, -1) {
		if len(m) > 1 {
			components = append(components, "jest_fail:"+strings.TrimSpace(m[1]))
		}
	}

	// 2. Extract error classes and core exception lines
	for _, m := range errorClassRegex.FindAllStringSubmatch(logContent, -1) {
		if len(m) > 2 {
			errClass := m[1]
			errDetail := normalizeVolatiles(strings.TrimSpace(m[2]))
			components = append(components, fmt.Sprintf("err:%s:%s", errClass, errDetail))
		}
	}

	// 3. Fallback normalization if no structured test matches were extracted
	if len(components) == 0 {
		normalized := normalizeVolatiles(logContent)
		lines := strings.Split(normalized, "\n")
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if trimmed != "" && (strings.Contains(trimmed, "fail") || strings.Contains(trimmed, "error") || strings.Contains(trimmed, "Error")) {
				components = append(components, "raw:"+trimmed)
			}
		}
		if len(components) == 0 && len(lines) > 0 {
			components = append(components, "raw:"+strings.TrimSpace(lines[0]))
		}
	}

	sort.Strings(components)
	canonical := strings.Join(components, "\n")

	hasher := sha256.New()
	hasher.Write([]byte(canonical))
	hash = hex.EncodeToString(hasher.Sum(nil))[:16]

	// Human-readable summary for reporting & directives
	summary = buildSummaryFromComponents(components)
	return hash, summary
}

func normalizeVolatiles(s string) string {
	s = memAddrRegex.ReplaceAllString(s, "0xADDR")
	s = durationRegex.ReplaceAllString(s, "0.0s")
	s = lineNumRegex.ReplaceAllString(s, ":LINE")
	s = turnNumRegex.ReplaceAllString(s, "Turn N")
	s = strikeRegex.ReplaceAllString(s, "strike N")
	return s
}

func buildSummaryFromComponents(components []string) string {
	if len(components) == 0 {
		return "Unknown error state"
	}
	var preview []string
	for i, c := range components {
		if i >= 3 {
			preview = append(preview, fmt.Sprintf("(+ %d more)", len(components)-3))
			break
		}
		preview = append(preview, c)
	}
	return strings.Join(preview, "; ")
}

// RecordFailure inspects the failure log for turn N, updates history, and detects cycles.
func (cb *OscillationCircuitBreaker) RecordFailure(turn int, failureLog string) OscillationDecision {
	hash, summary := cb.Fingerprint(failureLog)

	currentState := FailureState{
		Turn:    turn,
		Hash:    hash,
		Summary: summary,
	}

	// Check for periodic cycle matching previous states
	n := len(cb.history)
	cyclePeriod := 0
	var matchingState *FailureState

	for p := 2; p <= cb.maxCyclePeriod && p <= n; p++ {
		prev := cb.history[n-p]
		if prev.Hash == hash {
			// Ensure it's not complete stagnation (all intermediate states identical)
			isOscillation := false
			for k := n - p + 1; k < n; k++ {
				if cb.history[k].Hash != hash {
					isOscillation = true
					break
				}
			}
			if isOscillation {
				cyclePeriod = p
				matchingState = &prev
				break
			}
		}
	}

	cb.history = append(cb.history, currentState)
	if len(cb.history) > cb.maxWindow {
		cb.history = cb.history[len(cb.history)-cb.maxWindow:]
	}

	if cyclePeriod > 0 && matchingState != nil {
		stateA := matchingState.Summary
		stateB := cb.history[n-1].Summary

		// If reconciliation has not yet been attempted, issue a targeted reconciliation directive
		if cb.reconciliationsCount < cb.maxReconciliations {
			cb.reconciliationsCount++
			directive := cb.buildReconciliationDirective(cyclePeriod, matchingState.Turn, turn, stateA, stateB)
			return OscillationDecision{
				Action:    OscillationActionReconcile,
				Period:    cyclePeriod,
				Reason:    fmt.Sprintf("period-%d failure oscillation detected between Turn %d and Turn %d", cyclePeriod, matchingState.Turn, turn),
				Directive: directive,
				StateA:    stateA,
				StateB:    stateB,
			}
		}

		// Reconciliation was already attempted and cycle persisted: trip the circuit
		return OscillationDecision{
			Action: OscillationActionTrip,
			Period: cyclePeriod,
			Reason: fmt.Sprintf(
				"persistent period-%d failure oscillation detected between Turn %d and Turn %d after reconciliation attempt (State A: %s vs State B: %s)",
				cyclePeriod, matchingState.Turn, turn, stateA, stateB,
			),
			StateA: stateA,
			StateB: stateB,
		}
	}

	return OscillationDecision{
		Action: OscillationActionContinue,
	}
}

func (cb *OscillationCircuitBreaker) buildReconciliationDirective(period, prevTurn, curTurn int, stateA, stateB string) string {
	var sb strings.Builder
	sb.WriteString("\n=== 🚨 OSCILLATION CIRCUIT BREAKER: CONFLICT RECONCILIATION DIRECTIVE ===\n")
	fmt.Fprintf(&sb, "A repetitive period-%d oscillation cycle was detected across recent turns:\n", period)
	fmt.Fprintf(&sb, "- Failure Mode A (Turn %d): %s\n", prevTurn, stateA)
	fmt.Fprintf(&sb, "- Failure Mode B (Turn %d): %s\n\n", curTurn, stateB)
	sb.WriteString("DIAGNOSIS:\n")
	sb.WriteString("You are alternating between contradictory test or implementation expectations.\n")
	sb.WriteString("Modifying source code back and forth will NEVER pass all test suites simultaneously.\n\n")
	sb.WriteString("MANDATORY CORRECTIVE ACTION:\n")
	sb.WriteString("1. Inspect the conflicting test files and assertions corresponding to Failure Modes A and B.\n")
	sb.WriteString("2. Identify the authoritative canonical test suite (e.g. tests/unit/ or tests/integration/) and identify obsolete or conflicting test suites.\n")
	sb.WriteString("3. If an older test file imposes contradictory legacy requirements, DELETE IT using 'delete_file' (e.g. {\"path\": \"tests/test_legacy.py\"}).\n")
	sb.WriteString("4. Harmonize remaining test assertions so all tests adhere to one consistent public contract.\n")
	sb.WriteString("========================================================================\n\n")
	return sb.String()
}
