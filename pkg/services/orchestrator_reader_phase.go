package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	ctx, span := telemetry.Tracer().Start(ctx, "RunReaderPhase",
		trace.WithAttributes(
			attribute.String("task.id", task.ID),
			attribute.String("role", role),
		))
	defer span.End()

	var gatheredContext []string

	slicer := NewContextSlicer(o.cfg.Context)

	var projectPath string
	if state != nil {
		projectPath = state.ProjectPath
	}

	// Purge ephemeral cache directories and temporary files before context packing
	if projectPath != "" {
		_ = SanitizeWorkspace(projectPath, o.cfg.ExcludePaths...)
	}

	// Always append workspace file tree and manifests to prevent file duplication and import mismatch
	var availableFilesMsg string
	var rawWorkspaceFiles []string

	isGit := false
	if files, err := o.git.Run(ctx, false, "ls-files"); err == nil {
		isGit = true
		lines := strings.Split(files, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				rawWorkspaceFiles = append(rawWorkspaceFiles, line)
			}
		}
	} else if projectPath != "" {
		// Fallback when git ls-files fails (e.g. non-git workspace or unit test sandbox)
		_ = filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				if info != nil && info.IsDir() {
					name := info.Name()
					if name == ".git" || name == ".noctifab" || name == "node_modules" || name == "target" || name == ".venv" {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if rel, rErr := filepath.Rel(projectPath, path); rErr == nil {
				rawWorkspaceFiles = append(rawWorkspaceFiles, rel)
			}
			return nil
		})
	}

	filteredWorkspaceFiles := FilterRelevantFiles(projectPath, rawWorkspaceFiles, o.cfg.ExcludePaths)
	if isGit && len(filteredWorkspaceFiles) > 0 {
		availableFilesMsg = fmt.Sprintf("Workspace file structure:\n%s", strings.Join(filteredWorkspaceFiles, "\n"))
		gatheredContext = append(gatheredContext, availableFilesMsg)
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

	// Deterministic AST & Import Graph Walker: resolve target files, imported interfaces/types, and test callers
	walker := NewImportGraphWalker()
	graphCtx := walker.GatherContext(state.ProjectPath, task.TargetFiles, task.Title, task.Description, filteredWorkspaceFiles, slicer)
	if len(graphCtx) > 0 {
		fmt.Printf("Orchestrator: [Reader] deterministic AST import graph resolved %d file(s) for role %s\n", len(graphCtx), role)
		gatheredContext = append(gatheredContext, graphCtx...)
	}

	return gatheredContext
}
