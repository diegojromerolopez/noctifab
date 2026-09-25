package services

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var (
	ErrRepairFailed = errors.New("all repair attempts failed to fix the issue")
)

type FailureCategory int

const (
	FailureUnknown   FailureCategory = iota
	FailureTestLogic FailureCategory = iota
	FailureTimeout   FailureCategory = iota
	FailureCompile   FailureCategory = iota
	FailureSandbox   FailureCategory = iota
)

func (fc FailureCategory) String() string {
	return [...]string{"unknown", "test_logic", "timeout", "compile", "sandbox"}[fc]
}

func CategorizeFailureLog(log string) FailureCategory {
	lower := strings.ToLower(log)
	switch {
	case strings.Contains(lower, "no output produced within idle timeout"),
		strings.Contains(lower, "max wall-clock duration exceeded"),
		strings.Contains(lower, "command killed"):
		return FailureTimeout
	case strings.Contains(lower, "sandbox violation") ||
		strings.Contains(lower, "sandbox toolchain") ||
		strings.Contains(lower, "toolchain binary") ||
		strings.Contains(lower, "compiler not found") ||
		strings.Contains(lower, "gcc not found") ||
		strings.Contains(lower, "command not found") ||
		strings.Contains(lower, "not found in $path") ||
		strings.Contains(lower, "not found in %path%") ||
		strings.Contains(lower, "executable file not found") ||
		strings.Contains(lower, "exit status 127") ||
		strings.Contains(lower, ": not found") ||
		strings.Contains(lower, "cannot execute binary file") ||
		strings.Contains(lower, "is not recognized as an internal or external command") ||
		strings.Contains(lower, "cannot connect to the docker daemon") ||
		strings.Contains(lower, "is the docker daemon running") ||
		(strings.Contains(lower, "failed with:") && strings.Contains(lower, "not found")):
		return FailureSandbox
	case strings.Contains(lower, "compile error"),
		strings.Contains(lower, "syntax error"),
		strings.Contains(lower, "compilation error"):
		return FailureCompile
	case strings.Contains(lower, "error:"),
		strings.Contains(lower, "fail:"),
		strings.Contains(lower, "traceback"):
		return FailureTestLogic
	default:
		return FailureUnknown
	}
}

// repairPromptHead is the static prefix of the Repair Agent role body.
const repairPromptHead = `You are a software factory automation agent operating in a restricted workspace sandbox.
You must respond ONLY with a single JSON block. Do not include conversational markdown text or code fences (like ` + "`" + `json` + "`" + ` or ` + "`" + `) outside the JSON. All keys and string values in the JSON MUST be enclosed in double quotes (\""); never use single quotes (') for JSON strings or keys.

You are acting as the Repair Agent.
Your task is to fix the compilation error, linter offense, test failure, or watchdog timeout that is currently preventing the validation suite from passing.

Task Details & Failure Context:
`

// repairPromptTail is the static suffix of the Repair Agent role body: rules,
// tool list, and the JSON output schema. It is kept as a separate constant so
// the compaction layer can be told to never rewrite it
// (domain.WithUncompactableTail).
const repairPromptTail = `

CRITICAL:
1. TARGET FAILING FILES IMMEDIATELY: Read the failure output carefully and directly edit the failing file (e.g. Makefile or broken source file) indicated in the error trace. Avoid exploratory directory browsing when the failing path is already provided.
2. You may receive multiple turns. If the error is still present, you will be given the new failure output and another turn. Fix the issue immediately by editing or writing the necessary files.
3. All code written/modified MUST compile cleanly and comply with the project's formatting and linter guidelines.
4. Apply aggressive self-healing: fix any errors directly. Do not hesitate to overwrite or rewrite files to make them compile/validate correctly.
5. If you modify or write code that introduces references to new library or package features, you MUST ensure that all corresponding imports, headers, namespaces, or dependencies are correctly declared or included in the source file to prevent compiler, linter, or interpreter errors.
6. DIAGNOSTIC PROBE SCAFFOLDING & DEBUG LOOP: When diagnosing timeouts, socket/port connection drops, daemon liveness, or mysterious failures, DO NOT speculate in conversational text. Author an executable diagnostic probe script or test (e.g. under tests/) using write_file or invoke check_socket / check_http / validate_manifest. Run the probe to capture exact OS-level ground truth (ECONNREFUSED, ETIMEDOUT, exit codes) and iterate in a programmatic debug loop until the issue is fixed.
7. STRUCTURED DATA & SCRIPT-FIRST REFLEX: When dealing with structured data (JSON, YAML, TOML, CSV, schemas, ASTs, SQL, or tabular records), ASK YOURSELF if writing and executing an automated script (e.g. in Python or standard scripting tools) is better and more reliable than manual line-by-line editing. If so, write the script with write_file, execute it, and debug it iteratively.

You may use the following tools:
- read_file: read the contents of a file. Args: {"path": "relative/path/to/file"}
- write_file: create a new file or overwrite an existing one. Args: {"path": "relative/path/to/file", "content": "file content"}
- edit_file: modify an existing file. Args: {"path": "relative/path/to/file", "target_content": "exact code block to replace (must match the file content exactly; never include '[TRUNCATED]' or other placeholders)", "replacement_content": "new code block"}
- list_directory: list directory contents. Args: {"path": "relative/path/to/dir"}
- find_files: search for files. Args: {"pattern": "*"}
- grep_search: search for a pattern in files. Args: {"query": "search_term"}
- check_socket: test TCP/UDP connection, port ping/pong matching, and latency. Args: {"host": "localhost", "port": 6379, "timeout_seconds": 2}
- check_http: test HTTP endpoint status code, headers, and body response. Args: {"url": "http://localhost:8080/health", "method": "GET", "expected_status": 200}
- validate_manifest: validate package manifest dependencies (Cargo.toml, pyproject.toml, go.mod, package.json). Args: {"manifest_path": "Cargo.toml"}
- run_tests: run the project's tests to verify correctness. Args: {}
- run_linter: run the project's linter check in the sandbox workspace to verify syntax and style. Args: {}
- noop: call this when the failure is resolved. Args: {}

Return format:
{
  "reasoning": "Detailed technical rationale explaining your next step",
  "actions": [
    {
      "tool": "tool_name",
      "args": {
         "arg_name": "value"
      }
    }
  ]
}
`

// wrapRepairPrompt wraps the raw diagnostic details in the Repair Agent role
// body (persona, rules, tool list, JSON output schema). The repair prompt is
// deliberately hardcoded (no customization surface): the repair flow is
// dormant protocol machinery (see CUSTOM_PROMPTS.md §1.2).
func wrapRepairPrompt(details string) string {
	return repairPromptHead + details + repairPromptTail
}

func buildDiagnosticPrompt(title, description string, watchdogErr error, output string, category FailureCategory) string {
	switch category {
	case FailureTimeout:
		return fmt.Sprintf(`The test suite hung and was forcefully terminated by the watchdog.

Task: %s - %s

Watchdog error: %v

Last stdout output before timeout:
%s

This usually indicates:
- An infinite loop or deadlock
- An unjoined non-daemon thread
- A blocking operation (wait/sleep) that is never unblocked
- A resource leak exhausting file descriptors
- A daemon or socket connection blocking indefinitely

DIAGNOSTIC PROBE SCAFFOLDING DIRECTIVE:
DO NOT speculate why the system timed out.
1. If this is a daemon, socket, or network liveness failure, author an executable diagnostic probe script (e.g. under tests/) or invoke check_socket / check_http to verify port connectivity and responses.
2. If this is a deadlock or blocking wait, add targeted timeout assertions or isolate the hanging routine.
3. Focus on making the code terminate cleanly and deterministically.
`, title, description, watchdogErr, output)

	case FailureCompile:
		return fmt.Sprintf(`The compilation failed with error(s).

Task: %s - %s

Compilation error: %v

%s

Analyze the structured compilation errors above and fix the target files at the specified line numbers immediately.
`, title, description, watchdogErr, FormatStructuredErrorFeedback(output))

	case FailureTestLogic:
		return fmt.Sprintf(`The test suite execution failed with assertion or logic errors.

Task: %s - %s

Test validation error: %v

%s

Analyze the structured test failure(s) above and fix the target implementation or tests immediately.
`, title, description, watchdogErr, FormatStructuredErrorFeedback(output))

	default:
		return fmt.Sprintf(`The test suite validation failed.

Task: %s - %s

Validation error: %v

Output:
%s

Analyze the output and fix the implementation or tests immediately. Rewrite any files that need changes.
`, title, description, watchdogErr, output)
	}
}

func buildRetryPrompt(prevPrompt, testOutput string, testErr error, category FailureCategory, toolOutputs []string) string {
	msg := "fix the hang/deadlock"
	if category == FailureCompile {
		msg = "resolve the compilation error(s)"
	} else if category == FailureTestLogic {
		msg = "resolve the test failure(s)"
	} else if category != FailureTimeout {
		msg = "resolve the test validation failure(s)"
	}

	var toolOutputsBlock string
	if len(toolOutputs) > 0 {
		toolOutputsBlock = fmt.Sprintf("\n\nResults of tools executed in the previous attempt:\n%s", strings.Join(toolOutputs, "\n---\n"))
	}

	return fmt.Sprintf(`%s%s

The fix attempt was made but validation still failed:

Output:
%s

Error: %v

Please try a different approach to %s.
DIAGNOSTIC MANDATE: If the failure persists or the cause is unclear, author an executable diagnostic probe script (e.g. under tests/) or call check_socket / check_http / validate_manifest to isolate empirical OS-level ground truth.
In case of any persistent or unresolvable error, force a solution (even if simplified or fallback) to ensure the code compiles cleanly and passes all tests. Leaving a broken or non-compiling build is unacceptable.
`, prevPrompt, toolOutputsBlock, testOutput, testErr, msg)
}


type RepairResult struct {
	Success    bool
	Output     string
	FixedCode  bool
	Attempts   int
	FailureLog string
}

type WatchdogRepair struct {
	llmClient     domain.LLMClient
	maxRetries    int
	sandbox       Sandbox
	tools         map[string]Tool
	evaluator     *TestValidator
	fingerprinter *ErrorFingerprinter
}

// NewWatchdogRepair initializes a new WatchdogRepair service.
func NewWatchdogRepair(llmClient domain.LLMClient, sandbox Sandbox, tools map[string]Tool, evaluator *TestValidator) *WatchdogRepair {
	maxRetries := 10
	mergedTools := make(map[string]Tool)
	for k, v := range tools {
		mergedTools[k] = v
	}
	if _, ok := mergedTools["check_socket"]; !ok {
		mergedTools["check_socket"] = &CheckSocketTool{}
	}
	if _, ok := mergedTools["check_http"]; !ok {
		mergedTools["check_http"] = &CheckHTTPTool{}
	}
	if _, ok := mergedTools["validate_manifest"]; !ok {
		mergedTools["validate_manifest"] = &ValidateManifestTool{}
	}
	return &WatchdogRepair{
		llmClient:     llmClient,
		maxRetries:    maxRetries,
		sandbox:       sandbox,
		tools:         mergedTools,
		evaluator:     evaluator,
		fingerprinter: NewErrorFingerprinter(1),
	}
}

// AttemptRepair runs the LLM-driven repair loop for a hung or failing test
// suite: categorize the failure, send the diagnostic prompt, execute the
// returned tool actions, re-validate, and retry with fresh failure context
// until the suite passes or maxRetries is exhausted.
//
// DORMANT: WatchdogRepair is injected into the orchestrator in start.go and
// serve.go, but no production code path invokes AttemptRepair — only tests
// do. Wire it into the task-failure path or remove it; tracked in
// https://github.com/diegojromerolopez/noctifab/issues/15.
func (wr *WatchdogRepair) AttemptRepair(
	ctx context.Context,
	state *domain.State,
	task domain.Task,
	watchdogOutput string,
	watchdogErr error,
) (*RepairResult, error) {
	category := CategorizeFailureLog(watchdogOutput)
	diagPrompt := buildDiagnosticPrompt(task.Title, task.Description, watchdogErr, watchdogOutput, category)

	// Compaction must never rewrite the rules/tool-list/JSON-schema suffix.
	repairCtx := domain.WithUncompactableTail(ctx, len(repairPromptTail))

	var lastTestOutput string
	for attempt := 0; attempt < wr.maxRetries; attempt++ {
		resp, err := wr.llmClient.Complete(repairCtx, wrapRepairPrompt(diagPrompt))
		if err != nil {
			return nil, fmt.Errorf("repair LLM call failed: %w", err)
		}

		var toolOutputs []string
		for _, action := range resp.Actions {
			if tool, ok := wr.tools[action.Tool]; ok {
				fmt.Printf("Orchestrator: Repair action: tool=%s args=%v\n", action.Tool, action.Args)
				out, err := tool.Execute(ctx, state, action.Args)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Repair tool %s failed: %v\n", action.Tool, err)
					toolOutputs = append(toolOutputs, fmt.Sprintf("Action: tool=%s args=%v failed: %v\nOutput: %s", action.Tool, action.Args, err, out))
				} else {
					toolOutputs = append(toolOutputs, fmt.Sprintf("Action: tool=%s args=%v succeeded.\nOutput: %s", action.Tool, action.Args, out))
					if action.Tool == "write_file" || action.Tool == "edit_file" {
						wr.runFormatterIfConfigured(ctx, state)
					}
				}
			} else {
				toolOutputs = append(toolOutputs, fmt.Sprintf("Action: unknown tool %s", action.Tool))
			}
		}

		var passed bool
		var testOutput string
		var testErr error
		if wr.evaluator != nil {
			var err error
			passed, testOutput, err = wr.evaluator.ValidateTask(ctx, state, task)
			lastTestOutput = testOutput
			if err != nil {
				testErr = err
			} else if !passed {
				testErr = fmt.Errorf("validation failed: %s", category)
			}
		} else {
			testOutput, testErr = wr.sandbox.RunCommand(ctx, state.ProjectPath, "", "")
			passed = testErr == nil
			lastTestOutput = testOutput
		}

		if passed {
			fmt.Printf("🛠️  [Watchdog Repair Success] Task %s (%s) repaired successfully on attempt %d/%d!\n", task.ID, task.Title, attempt+1, wr.maxRetries)
			return &RepairResult{
				Success:   true,
				Output:    testOutput,
				FixedCode: true,
				Attempts:  attempt + 1,
			}, nil
		}

		if wr.fingerprinter != nil && testOutput != "" {
			if isLoop, _, diag := wr.fingerprinter.RecordAndCheckLoop(testOutput); isLoop {
				fmt.Printf("⚠️  [Deterministic Anti-Loop Gate] Repair attempt produced identical failure signature\n")
				diagPrompt += fmt.Sprintf("\n\n⚠️ [DETERMINISTIC ANTI-LOOP GATE] %s. Modifying code in this manner reproduces the exact same failure. Repeating this edit is forbidden; you MUST try an entirely different fix.", diag)
			}
		}

		diagPrompt = buildRetryPrompt(diagPrompt, testOutput, testErr, category, toolOutputs)
	}

	fmt.Printf("❌ [Watchdog Repair Exhausted] Task %s (%s) failed repair after %d attempts (category: %s)\n", task.ID, task.Title, wr.maxRetries, category.String())

	return &RepairResult{
		Success:    false,
		Output:     lastTestOutput,
		Attempts:   wr.maxRetries,
		FailureLog: fmt.Sprintf("all repair attempts failed to resolve the %s failure", category),
	}, nil
}

func (wr *WatchdogRepair) runFormatterIfConfigured(ctx context.Context, state *domain.State) {
	if wr.evaluator != nil {
		if wr.evaluator.Formatter != nil {
			fmt.Printf("Orchestrator: Running formatter command (repair): %s\n", wr.evaluator.Formatter.GetCommand())
			_, _ = wr.evaluator.Formatter.Format(ctx, state.ProjectPath)
			return
		}
		if wr.evaluator.FormatterCommand != "" {
			fmt.Printf("Orchestrator: Running formatter command (repair): %s\n", wr.evaluator.FormatterCommand)
			_, _ = wr.sandbox.RunCommand(ctx, state.ProjectPath, wr.evaluator.FormatterCommand, "")
		}
	}
}
