package services

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

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
