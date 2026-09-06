package services

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

// ExecuteSpike executes the greenfield spike prototyping phase.
// If the workspace is greenfield (no existing source files) and spike is enabled,
// it prompts a single prioritized model to generate a minimal compiling walking skeleton
// in one batch turn, writes the files, and commits them to git before handoff to the PM.
func ExecuteSpike(
	ctx context.Context,
	projectPath string,
	cfg *config.Config,
	llmClient domain.LLMClient,
	renderer PromptRenderer,
	reporter domain.ExecutionReporter,
) (bool, error) {
	if cfg == nil || !cfg.Agents.Spike.IsEnabled() {
		return false, nil
	}

	// 1. Greenfield check: if source files already exist, skip the spike phase
	existingFiles, err := ScanLegacyFiles(projectPath)
	if err == nil && len(existingFiles) > 0 {
		fmt.Printf("ℹ [Spike] Existing source code detected (%d files); skipping spike phase.\n", len(existingFiles))
		return false, nil
	}

	// 2. Read SPEC.md
	specPath := filepath.Join(projectPath, "SPEC.md")
	specBytes, err := os.ReadFile(specPath)
	if err != nil || len(strings.TrimSpace(string(specBytes))) == 0 {
		return false, nil
	}

	fmt.Printf("🚀 [Spike] Greenfield workspace detected. Executing Spike Phase to build initial walking skeleton...\n")
	spikeStart := time.Now()

	if reporter != nil {
		reporter.Observe(ctx, domain.ExecutionEvent{
			Kind:      domain.EventPhaseStarted,
			Name:      "spike_prototyping",
			AgentRole: "spike",
			At:        spikeStart.UTC(),
		})
	}

	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}

	specText := string(specBytes)
	if cfg != nil && cfg.Context.GetCompactionMode() != "none" {
		specText = llm.CompactMarkdownSpecWithMode(specText, cfg.Context.GetCompactionMode())
	}

	rendered, err := renderer.Render(prompts.AgentSpike, "generate", prompts.SpikePromptData{
		Spec: specText,
	})
	if err != nil {
		return false, fmt.Errorf("failed to render spike prompt: %w", err)
	}

	spikeCtx := llm.WithRoleContext(ctx, "spike")
	spikeCtx = domain.WithUncompactableTail(spikeCtx, len(rendered.Contract))

	timeoutSec := cfg.Agents.Spike.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	callCtx, cancel := context.WithTimeout(spikeCtx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	resp, err := llmClient.Complete(callCtx, rendered.Full())
	if err != nil {
		durMS := time.Since(spikeStart).Milliseconds()
		if reporter != nil {
			reporter.Observe(ctx, domain.ExecutionEvent{
				Kind:           domain.EventPhaseFinished,
				Name:           "spike_prototyping",
				AgentRole:      "spike",
				DurationMillis: &durMS,
				Outcome:        domain.OutcomeFailed,
				At:             time.Now().UTC(),
			})
		}
		return false, fmt.Errorf("spike model completion failed: %w", err)
	}

	filesWritten := 0
	if resp != nil {
		for _, act := range resp.Actions {
			switch act.Tool {
			case "write_files":
				filesMap, ok := act.Args["files"].(map[string]any)
				if !ok {
					// Also check map[string]string
					if fm, ok2 := act.Args["files"].(map[string]string); ok2 {
						filesMap = make(map[string]any, len(fm))
						for k, v := range fm {
							filesMap[k] = v
						}
					}
				}
				for path, cVal := range filesMap {
					contentStr, ok := cVal.(string)
					if !ok || strings.TrimSpace(path) == "" {
						continue
					}
					cleanPath := filepath.Clean(strings.TrimSpace(path))
					if strings.HasPrefix(cleanPath, "..") || filepath.IsAbs(cleanPath) {
						continue
					}
					fullPath := filepath.Join(projectPath, cleanPath)
					if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
						continue
					}
					if err := os.WriteFile(fullPath, []byte(contentStr), 0644); err == nil {
						filesWritten++
					}
				}
			case "write_file":
				path, _ := act.Args["path"].(string)
				content, _ := act.Args["content"].(string)
				if strings.TrimSpace(path) != "" {
					cleanPath := filepath.Clean(strings.TrimSpace(path))
					if !strings.HasPrefix(cleanPath, "..") && !filepath.IsAbs(cleanPath) {
						fullPath := filepath.Join(projectPath, cleanPath)
						if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err == nil {
							if err := os.WriteFile(fullPath, []byte(content), 0644); err == nil {
								filesWritten++
							}
						}
					}
				}
			}
		}
	}

	durMS := time.Since(spikeStart).Milliseconds()
	if reporter != nil {
		outcome := domain.OutcomeSuccess
		if filesWritten == 0 {
			outcome = domain.OutcomeFailed
		}
		reporter.Observe(ctx, domain.ExecutionEvent{
			Kind:           domain.EventPhaseFinished,
			Name:           "spike_prototyping",
			AgentRole:      "spike",
			DurationMillis: &durMS,
			Outcome:        outcome,
			At:             time.Now().UTC(),
		})
	}

	if filesWritten == 0 {
		return false, fmt.Errorf("spike model did not produce any valid file writing actions")
	}

	// Commit initial spike solution to git
	commitSpikeToGit(projectPath)

	fmt.Printf("✅ [Spike] Minimal walking skeleton implemented (%d files written in %v). Handing off to Product Manager.\n", filesWritten, time.Since(spikeStart).Round(time.Millisecond))
	return true, nil
}

func commitSpikeToGit(projectPath string) {
	cmdAdd := exec.Command("git", "add", ".")
	cmdAdd.Dir = projectPath
	_ = cmdAdd.Run()

	cmdCommit := exec.Command("git", "commit", "-m", "feat: initial spike solution walking skeleton")
	cmdCommit.Dir = projectPath
	_ = cmdCommit.Run()
}
