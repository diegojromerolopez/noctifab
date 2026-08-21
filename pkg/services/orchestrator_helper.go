package services

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

func (o *Orchestrator) recordTokenUsage(ctx context.Context, prompt string, resp *domain.LLMResponse) {
	if o == nil || o.repo == nil || resp == nil {
		return
	}
	tokens := llm.EstimateUsageTokens(prompt, resp)
	if tokens <= 0 {
		return
	}
	_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
		st.Metadata.TotalTokensUsed += tokens
		return nil
	})
}

func (o *Orchestrator) registerAgentStart(ctx context.Context, role string, taskID string) {
	agentID := fmt.Sprintf("agent-%s-%s", role, taskID)
	name := fmt.Sprintf("%s-%s", role, taskID)
	if o.observer != nil {
		o.observer.Observe(ctx, domain.ExecutionEvent{
			Kind:              domain.EventAgentStarted,
			AgentInvocationID: agentID,
			AgentRole:         role,
			TaskID:            taskID,
			At:                time.Now().UTC(),
		})
	}
	updateErr := o.updateStateWithRetry(ctx, func(st *domain.State) error {
		found := false
		for i := range st.ActiveAgents {
			if st.ActiveAgents[i].ID == agentID {
				st.ActiveAgents[i].Status = domain.AgentWorking
				st.ActiveAgents[i].TaskID = taskID
				st.ActiveAgents[i].StartedAt = time.Now()
				st.ActiveAgents[i].CompletedAt = time.Time{}
				found = true
				break
			}
		}
		if !found {
			st.ActiveAgents = append(st.ActiveAgents, domain.Agent{
				ID:        agentID,
				Name:      name,
				Role:      domain.AgentRole(strings.ToUpper(role)),
				Status:    domain.AgentWorking,
				TaskID:    taskID,
				StartedAt: time.Now(),
			})
		}
		return nil
	})
	if updateErr != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: failed to register agent start for role %s task %s: %v\n", role, taskID, updateErr)
	}
}

func (o *Orchestrator) registerAgentComplete(ctx context.Context, role string, taskID string, err error) {
	agentID := fmt.Sprintf("agent-%s-%s", role, taskID)
	if o.observer != nil {
		outcome := domain.OutcomeSuccess
		if err != nil {
			outcome = domain.OutcomeFailed
		}
		o.observer.Observe(ctx, domain.ExecutionEvent{
			Kind:              domain.EventAgentFinished,
			AgentInvocationID: agentID,
			AgentRole:         role,
			TaskID:            taskID,
			Outcome:           outcome,
			At:                time.Now().UTC(),
		})
	}
	updateErr := o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.ActiveAgents {
			if st.ActiveAgents[i].ID == agentID {
				st.ActiveAgents[i].Status = domain.AgentCompleted
				st.ActiveAgents[i].CompletedAt = time.Now()
				if err != nil {
					st.ActiveAgents[i].LastError = err.Error()
				} else {
					st.ActiveAgents[i].LastError = ""
				}
				break
			}
		}
		return nil
	})
	if updateErr != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: failed to register agent completion for role %s task %s: %v\n", role, taskID, updateErr)
	}
}

// readerPromptTail is the static suffix of the Reader (context gathering)
// prompt: inspection tool list and JSON output schema. Kept as a separate
// constant so the compaction layer can be told to never rewrite it
// (domain.WithUncompactableTail).
const readerPromptTail = `You may call the following inspection tools:
- read_file: read the contents of a file. Args: {"path": "relative/path/to/file"}
- list_directory: list directory contents. Args: {"path": "relative/path/to/dir"}
- find_files: search for files. Args: {"pattern": "*.py"}
- grep_search: search for a pattern in files. Args: {"query": "search_term"}
- noop: call this if you have enough context and do not need to read any more files.

Return format:
{
  "reasoning": "Explain what context you need to gather",
  "actions": [
    {
      "tool": "read_file",
      "args": {
        "path": "src/module.ext"
      }
    }
  ]
}
`

// RunReaderPhase runs the pre-step to collect workspace context before execution
func (o *Orchestrator) RunReaderPhase(ctx context.Context, role string, task domain.Task, state *domain.State) []string {
	var gatheredContext []string

	slicer := NewContextSlicer(o.cfg.Context)

	// Always append workspace file tree and manifests to prevent file duplication and import mismatch
	var availableFilesMsg string
	if files, err := o.git.Run(ctx, false, "ls-files"); err == nil {
		lines := strings.Split(files, "\n")
		var filtered []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, ".noctifab") || strings.HasPrefix(line, ".git") {
				continue
			}
			filtered = append(filtered, line)
		}
		if len(filtered) > 0 {
			availableFilesMsg = fmt.Sprintf("Workspace file structure:\n%s", strings.Join(filtered, "\n"))
			gatheredContext = append(gatheredContext, availableFilesMsg)
		}
	}

	// Read workspace project manifest if present
	manifestCandidates := []string{"Cargo.toml", "go.mod", "package.json", "pyproject.toml", "Makefile", "CMakeLists.txt"}
	rfTool, hasRf := o.registry.Get("read_file")
	if hasRf {
		for _, m := range manifestCandidates {
			mOut, mErr := rfTool.Execute(ctx, state, map[string]any{"path": m})
			if mErr == nil && strings.TrimSpace(mOut) != "" {
				gatheredContext = append(gatheredContext, fmt.Sprintf("Project Manifest (%s):\n```\n%s\n```", m, strings.TrimSpace(mOut)))
				break
			}
		}
	}

	// Heuristic Context Loading: automatically read target files if they exist to save an LLM turn
	if len(task.TargetFiles) > 0 && hasRf {
		for _, tf := range task.TargetFiles {
			if tf == "" {
				continue
			}
			args := map[string]any{"path": tf}
			out, err := rfTool.Execute(ctx, state, args)
			if err == nil && out != "" {
				slicedCtx := slicer.SliceFileContext(tf, out, "")
				gatheredContext = append(gatheredContext, slicedCtx)
			}
		}
		if len(gatheredContext) > 1 {
			fmt.Printf("Orchestrator: [Reader] role %s using heuristically loaded context for %d target file(s), skipping LLM call\n", role, len(task.TargetFiles))
			return gatheredContext
		}
	}

	// Fallback: Parse file paths directly from task.Description using regex before resorting to LLM
	filePathRegex := regexp.MustCompile(`[a-zA-Z0-9_\-/\.]+\.[a-zA-Z0-9]+`)
	matches := filePathRegex.FindAllString(task.Description, -1)
	for _, file := range matches {
		fullPath, err := resolveSandboxPath(state.ProjectPath, file)
		if err == nil {
			if content, err := os.ReadFile(fullPath); err == nil && len(content) > 0 {
				summary := string(content)
				if len(summary) > 2000 {
					summary = summary[:2000] + "\n... [TRUNCATED] ..."
				}
				gatheredContext = append(gatheredContext, fmt.Sprintf("Heuristically read file %q from description:\n```\n%s\n```", file, summary))
			}
		}
	}

	if len(gatheredContext) > 1 {
		fmt.Printf("Orchestrator: [Reader] role %s loaded file(s) from description heuristics, skipping LLM call\n", role)
		return gatheredContext
	}

	if files, err := o.git.Run(ctx, false, "ls-files"); err == nil {
		lines := strings.Split(files, "\n")
		var filtered []string
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			parts := strings.Split(line, "/")
			ignored := false
			for _, part := range parts {
				if part == ".noctifab" || part == ".git" {
					ignored = true
					break
				}
				for _, exp := range o.cfg.ExcludePaths {
					cleanExp := strings.Trim(exp, "/")
					if cleanExp != "" && part == cleanExp {
						ignored = true
						break
					}
				}
				if ignored {
					break
				}
			}
			if !ignored {
				filtered = append(filtered, line)
			}
		}
		availableFilesMsg = fmt.Sprintf("\nBelow is a list of all existing files in the repository:\n%s\n", strings.Join(filtered, "\n"))
	}

	prompt := fmt.Sprintf(`You are a software factory automation agent operating in a restricted workspace sandbox.
You must respond ONLY with a single JSON block. Do not include conversational markdown text or code fences (like `+"`"+`json or `+"`"+`) outside the JSON.

You are acting as the %s Agent in the Context Gathering phase.
Your objective is to inspect the workspace files and directories to gather necessary context before writing any code or tests.

Task Details:
Title: %s
Description: %s

Below is a list of target files for this task:
%v
%s
`, role, task.Title, task.Description, task.TargetFiles, availableFilesMsg) + readerPromptTail

	readerCtx := context.WithValue(ctx, AgentRoleKey, role)
	// Compaction must never rewrite the tool-list/JSON-schema suffix.
	readerCtx = domain.WithUncompactableTail(readerCtx, len(readerPromptTail))
	resp, err := o.llmClient.Complete(readerCtx, prompt)
	o.recordTokenUsage(ctx, prompt, resp)
	if err != nil {
		fmt.Printf("Orchestrator: Task [Reader] phase failed for role %s: %v. Continuing without extra context.\n", role, err)
		return nil
	}
	fmt.Printf("Orchestrator: [Reader] phase ok for role %s: actions=%d\n", role, len(resp.Actions))

	for _, action := range resp.Actions {
		if action.Tool == "noop" {
			continue
		}
		if action.Tool != "read_file" && action.Tool != "list_directory" && action.Tool != "find_files" && action.Tool != "grep_search" {
			continue
		}
		tool, ok := o.registry.Get(action.Tool)
		if ok {
			fmt.Printf("Orchestrator: [Reader] role %s executing tool: %s with args: %+v\n", role, action.Tool, action.Args)
			out, err := tool.Execute(ctx, state, action.Args)
			if err != nil {
				fmt.Printf("Orchestrator: [Reader] role %s tool %s failed: %v\n", role, action.Tool, err)
			} else {
				summary := out
				if len(summary) > 2000 {
					summary = summary[:2000] + "\n... [TRUNCATED] ..."
				}
				gatheredContext = append(gatheredContext, fmt.Sprintf("Inspection result of calling %s with args %+v:\n```\n%s\n```", action.Tool, action.Args, summary))
			}
		}
	}
	return gatheredContext
}

// RunTesterAgent runs the test writer agent for the task. action selects the
// tester prompt template (see the prompts package catalog: write, fix,
// refactor, write_breadth_first); feedback carries the generator's test-fix
// feedback for the fix action (empty otherwise).
func (o *Orchestrator) RunTesterAgent(ctx context.Context, task domain.Task, state *domain.State, fileContexts []string, action string, feedback string) {
	// Fail fast on unknown actions before doing any reader-phase work.
	if err := prompts.ValidateKey(prompts.AgentTester, action); err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Tester] invalid prompt action: %v\n", task.ID, err)
		o.registerAgentStart(ctx, "tester", task.ID)
		o.registerAgentComplete(ctx, "tester", task.ID, err)
		return
	}

	// Reader Phase: collect inspection context first! (Requirement 5)
	readerContexts := o.RunReaderPhase(ctx, "tester", task, state)

	var promptContext []string
	if len(fileContexts) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("Existing files context:\n%s", strings.Join(fileContexts, "\n\n")))
	}
	if len(readerContexts) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("Inspection context gathered:\n%s", strings.Join(readerContexts, "\n\n")))
	}
	if len(task.UserDirectives) > 0 {
		promptContext = append(promptContext, fmt.Sprintf("### 🎯 [USER HUMAN-IN-THE-LOOP STEERING DIRECTIVES]\n%s", strings.Join(task.UserDirectives, "\n")))
	}
	if task.Retries > 0 && task.FailureLog != "" {
		warning := "WARNING: The previous implementation/refactoring changes from the failed attempt have been preserved in the workspace files. You must inspect the existing code/tests, identify the bugs, and modify the files to fix the failures."
		summary := summarizeFailureLog(task.FailureLog)
		promptContext = append(promptContext, fmt.Sprintf("%s\n\nPrevious implementation attempt FAILED. Key failure details from the test run:\n%s\n\nFix the tests to address these specific errors.", warning, summary))
	}
	contextBlock := ""
	if len(promptContext) > 0 {
		contextBlock = "\n\n" + strings.Join(promptContext, "\n\n")
	}

	rendered, err := o.promptRenderer.Render(prompts.AgentTester, action, prompts.TaskPromptData{
		Title:             task.Title,
		Description:       task.Description,
		Context:           contextBlock,
		Feedback:          feedback,
		RecoveryDirective: task.RecoveryDirective,
		TargetFiles:       task.TargetFiles,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Tester] prompt rendering failed for action %q: %v\n", task.ID, action, err)
		o.registerAgentStart(ctx, "tester", task.ID)
		o.registerAgentComplete(ctx, "tester", task.ID, err)
		return
	}
	testPrompt := rendered.Full()

	testerCtx := context.WithValue(ctx, AgentRoleKey, "tester")
	// Compaction must never rewrite the output contract at the end of the prompt.
	testerCtx = domain.WithUncompactableTail(testerCtx, len(rendered.Contract))
	o.registerAgentStart(ctx, "tester", task.ID)

	currentPrompt := testPrompt
	maxTurns := iterationsOrDefault(o.cfg.TestersIterations)
	var lastErr error
	runTestsCalled := false
	diagCache := NewTaskDiagnosticCache(o.cfg.GetWorkspaceCache().IsEnabled())
	// consecutiveLinterFailures tracks back-to-back run_linter failures without
	// any file mutation in between. When it reaches 2, run_linter is skipped
	// for the remainder of this task to prevent the stale-cache lock-in spiral.
	consecutiveLinterFailures := 0
	linterDeferred := false

	for turn := 0; turn < maxTurns; turn++ {
		testResp, err := o.llmClient.Complete(testerCtx, currentPrompt)
		o.recordTokenUsage(ctx, currentPrompt, testResp)
		if err != nil {
			lastErr = err
			break
		}

		executed := 0
		blocked := 0
		var turnToolOutputs []string
		hasNoop := false

		for _, action := range testResp.Actions {
			if action.Tool == "noop" {
				hasNoop = true
				continue
			}
			if action.Tool == "run_tests" {
				runTestsCalled = true
			}
			fmt.Printf("Orchestrator: Task %s [Tester] action: tool=%s args=%+v\n", task.ID, action.Tool, action.Args)
			domainAction := domain.Action{
				Tool: action.Tool,
				Args: action.Args,
			}
			valRes, valErr := o.validator.Validate(testerCtx, domainAction, state)
			if valErr != nil || (valRes != nil && !valRes.Allowed) {
				blocked++
				reason := ""
				if valRes != nil {
					reason = valRes.Reason
				}
				fmt.Fprintf(os.Stderr, "Orchestrator: Task %s [Tester] action %s blocked: %s\n", task.ID, action.Tool, reason)
				turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s blocked by policy: %s", action.Tool, reason))
				continue
			}

			if cachedOut, cachedErr, hasCache := diagCache.TryGetCachedInspection(action.Tool, action.Args); hasCache {
				fmt.Printf("Orchestrator: Task %s [Tester] inspection action %s served from cache\n", task.ID, action.Tool)
				if cachedErr != nil {
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s failed: %v\nOutput: %s", action.Tool, cachedErr, cachedOut))
				} else {
					executed++
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s executed successfully. Output:\n%s", action.Tool, cachedOut))
				}
				continue
			}

			if cachedOut, cachedErr, hasCache := diagCache.TryGetCachedResult(action.Tool); hasCache {
				fmt.Printf("Orchestrator: Task %s [Tester] diagnostic action %s served from cache\n", task.ID, action.Tool)
				if cachedErr != nil {
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s failed: %v\nOutput: %s", action.Tool, cachedErr, cachedOut))
				} else {
					executed++
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s executed successfully. Output:\n%s", action.Tool, cachedOut))
				}
				continue
			}

			if action.Tool == "run_linter" && linterDeferred {
				msg := "[LINTER DEFERRED] run_linter skipped: linter failed twice consecutively without file changes. Focus on run_tests — tests are the primary quality gate. Linter cleanup will happen in a later pass."
				turnToolOutputs = append(turnToolOutputs, msg)
				continue
			}

			tool, ok := o.registry.Get(action.Tool)
			if ok {
				out, execErr := tool.Execute(testerCtx, state, action.Args)
				diagCache.OnToolExecuted(action.Tool, action.Args, out, execErr)
				fmt.Printf("🛠️  [Tool Executed] task=%s role=TESTER tool=%s success=%t\n", task.ID, action.Tool, execErr == nil)
				if execErr != nil {
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s failed: %v\nOutput: %s", action.Tool, execErr, out))
					// Track linter consecutive failures.
					if action.Tool == "run_linter" {
						consecutiveLinterFailures++
						if consecutiveLinterFailures >= 2 {
							linterDeferred = true
							fmt.Fprintf(os.Stderr, "⚠ [Tester] Linter failed %d consecutive times without file changes for task %s — deferring linter enforcement. Tests are the primary quality gate.\n", consecutiveLinterFailures, task.ID)
						}
					}
					if action.Tool != "run_linter" || !linterDeferred {
						hasNoop = false
					}
				} else {
					executed++
					turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Tool %s executed successfully. Output:\n%s", action.Tool, out))
					// Reset linter failure counter on any successful file mutation.
					switch action.Tool {
					case "write_file", "edit_file", "multi_replace_file_content", "delete_file":
						consecutiveLinterFailures = 0
					}
				}
			}
		}

		if hasNoop || len(testResp.Actions) == 0 {
			if !runTestsCalled {
				fmt.Printf("Orchestrator: Agent returned noop without executing run_tests; auto-triggering run_tests fallback for task %s\n", task.ID)
				runTestsTool, ok := o.registry.Get("run_tests")
				if ok {
					out, execErr := runTestsTool.Execute(testerCtx, state, map[string]any{})
					if execErr != nil {
						fmt.Printf("Orchestrator: Auto-triggered run_tests failed for task %s: %v. Rejecting noop.\n", task.ID, execErr)
						turnToolOutputs = append(turnToolOutputs, fmt.Sprintf("Action 'noop' rejected: auto-triggered run_tests failed: %v\nOutput:\n%s", execErr, out))
						hasNoop = false
					}
				}
			}
			if hasNoop {
				break
			}
		}

		// Append errors and tool outputs to the body for the next turn. The
		// non-overridable output contract stays at the END of the prompt so
		// the JSON schema is the last thing the model reads.
		currentPrompt = fmt.Sprintf("%s\n\nTOOL OUTPUTS FROM PREVIOUS TURN (turn %d/%d):\n%s\n\nBased on these outputs, take your next actions. If everything is done and verified, call noop. You have %d turns remaining.\n%s",
			rendered.Body, turn+1, maxTurns,
			joinCappedToolOutputs(turnToolOutputs),
			maxTurns-turn-1,
			rendered.Contract)
	}

	o.registerAgentComplete(ctx, "tester", task.ID, lastErr)
}
