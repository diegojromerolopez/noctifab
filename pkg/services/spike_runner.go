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
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
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
	ctx, span := telemetry.Tracer().Start(ctx, "ExecuteSpike",
		trace.WithAttributes(attribute.String("project_path", projectPath)))
	defer span.End()

	if cfg == nil || !cfg.Agents.Spike.IsEnabled() {
		return false, nil
	}

	// 1. Greenfield check: if source files already exist, skip the spike phase
	existingFiles, err := ScanLegacyFiles(projectPath)
	if err == nil && len(existingFiles) > 0 {
		fmt.Printf("ℹ [Spike] Existing source code detected (%d files); skipping spike phase.\n", len(existingFiles))
		return false, nil
	}

	// 2. Read specification
	specPath := ResolveSpecPath(projectPath)
	specBytes, err := os.ReadFile(specPath)
	if err != nil || len(strings.TrimSpace(string(specBytes))) == 0 {
		return false, nil
	}

	// Greenfield monorepo check: fullstack monorepos (client + server) routinely timeout in a single 180s spike turn
	if IsMonorepoSpec(string(specBytes)) {
		fmt.Printf("ℹ [Spike] Monorepo/Full-Stack architecture detected in specification; bypassing monolithic spike in favor of modular Product Manager user stories.\n")
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
	spikeCtx = llm.WithNonStreaming(spikeCtx)
	spikeCtx = domain.WithUncompactableTail(spikeCtx, len(rendered.Contract))
	spikeCtx = domain.WithCacheablePrefix(spikeCtx, len(rendered.Body))

	timeoutSec := cfg.Agents.Spike.TimeoutSeconds
	if timeoutSec <= 0 {
		timeoutSec = 180
	}
	var filesToApply map[string]string
	var candidates []SpikeCandidate

	if cfg.Agents.Spike.Hypotheses.IsEnabled() {
		candidates = GenerateSpikeHypotheses(spikeCtx, rendered.Full(), llmClient, cfg.Agents.Spike.Hypotheses, timeoutSec)
		filesToApply = ResolveSpikeHypotheses(candidates, cfg.Agents.Spike.Hypotheses.GetResolution())
	} else {
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
		filesToApply = extractFilesFromSpikeResponse(resp)
	}

	writtenPaths, compErr := applyAndVerifySpikeCandidate(ctx, projectPath, filesToApply)
	if compErr != nil && len(candidates) > 0 {
		fmt.Printf("⚠️  [Spike] Combined/primary hypothesis failed compile check (%v). Testing individual hypotheses...\n", compErr)
		for _, cand := range candidates {
			if !cand.Valid || len(cand.Files) == 0 {
				continue
			}
			writtenPaths, compErr = applyAndVerifySpikeCandidate(ctx, projectPath, cand.Files)
			if compErr == nil && len(writtenPaths) > 0 {
				fmt.Printf("✅ [Spike] Individual Hypothesis %d passed compile check (%d files).\n", cand.Index, len(writtenPaths))
				break
			}
		}
	}

	filesWritten := len(writtenPaths)
	durMS := time.Since(spikeStart).Milliseconds()
	if reporter != nil {
		outcome := domain.OutcomeSuccess
		if filesWritten == 0 || compErr != nil {
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

	if compErr != nil {
		fmt.Printf("⚠️  [Spike] Discarded uncompilable spike prototype (%v) to keep main branch clean.\n", compErr)
		return false, fmt.Errorf("spike prototype failed compilation gate: %w", compErr)
	}

	if filesWritten == 0 {
		return false, fmt.Errorf("spike model did not produce any valid file writing actions")
	}

	// Commit verified spike solution to git
	commitSpikeToGit(projectPath, writtenPaths)

	fmt.Printf("✅ [Spike] Minimal walking skeleton verified & committed (%d files written in %v). Handing off to Product Manager.\n", filesWritten, time.Since(spikeStart).Round(time.Millisecond))
	return true, nil
}

func applyAndVerifySpikeCandidate(ctx context.Context, projectPath string, filesToApply map[string]string) ([]string, error) {
	if len(filesToApply) == 0 {
		return nil, nil
	}

	var createdFiles []string
	overwrittenFiles := make(map[string][]byte)
	var writtenRelPaths []string

	for path, content := range filesToApply {
		cleanPath := filepath.Clean(strings.TrimSpace(path))
		if strings.HasPrefix(cleanPath, "..") || filepath.IsAbs(cleanPath) {
			continue
		}
		if isMakefile(cleanPath) {
			content = StandardizeMakefile(content)
		}
		fullPath := filepath.Join(projectPath, cleanPath)
		if prevBytes, err := os.ReadFile(fullPath); err == nil {
			overwrittenFiles[fullPath] = prevBytes
		} else {
			createdFiles = append(createdFiles, fullPath)
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err == nil {
			if err := os.WriteFile(fullPath, []byte(content), 0644); err == nil {
				writtenRelPaths = append(writtenRelPaths, cleanPath)
			}
		}
	}

	if len(writtenRelPaths) == 0 {
		return nil, nil
	}

	rootMakefilePath := filepath.Join(projectPath, "Makefile")
	if mfBytes, err := os.ReadFile(rootMakefilePath); err == nil {
		if _, tracked := overwrittenFiles[rootMakefilePath]; !tracked {
			createdHasMakefile := false
			for _, cf := range createdFiles {
				if cf == rootMakefilePath {
					createdHasMakefile = true
					break
				}
			}
			if !createdHasMakefile {
				overwrittenFiles[rootMakefilePath] = mfBytes
			}
		}
		stdMF := StandardizeMakefile(string(mfBytes))
		_ = os.WriteFile(rootMakefilePath, []byte(stdMF), 0644)
	}

	if err := VerifySpikeWorkspaceCompiles(ctx, projectPath, writtenRelPaths); err != nil {
		for _, cf := range createdFiles {
			_ = os.Remove(cf)
		}
		for fullPath, origBytes := range overwrittenFiles {
			_ = os.WriteFile(fullPath, origBytes, 0644)
		}
		return nil, err
	}

	return writtenRelPaths, nil
}

// VerifySpikeWorkspaceCompiles runs fast deterministic syntax and build checks on written Spike files.
func VerifySpikeWorkspaceCompiles(ctx context.Context, projectPath string, writtenRelPaths []string) error {
	sv := NewSyntaxValidator()
	if violations, err := sv.ValidateFiles(ctx, projectPath, writtenRelPaths); err != nil {
		return err
	} else if len(violations) > 0 {
		return fmt.Errorf("syntax error in %s: %s", violations[0].FilePath, violations[0].Message)
	}

	hasExt := func(ext string) bool {
		for _, p := range writtenRelPaths {
			if strings.EqualFold(filepath.Ext(p), ext) {
				return true
			}
		}
		return false
	}
	fileExists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(projectPath, rel))
		return err == nil
	}

	var buildCmd []string
	switch {
	case fileExists("go.mod") && hasExt(".go"):
		buildCmd = []string{"go", "build", "./..."}
	case fileExists("Cargo.toml") && hasExt(".rs"):
		buildCmd = []string{"cargo", "check", "--quiet"}
	case hasExt(".csproj") || hasExt(".sln") || hasExt(".cs"):
		if hasExt(".csproj") || hasExt(".sln") {
			buildCmd = []string{"dotnet", "build", "--nologo", "-v", "q"}
		}
	case fileExists("dune-project") && (hasExt(".ml") || hasExt(".mli")):
		buildCmd = []string{"dune", "build"}
	case fileExists("rebar.config") && hasExt(".erl"):
		buildCmd = []string{"rebar3", "compile"}
	}

	if len(buildCmd) > 0 {
		if _, lookErr := exec.LookPath(buildCmd[0]); lookErr == nil {
			bCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(bCtx, buildCmd[0], buildCmd[1:]...)
			cmd.Dir = projectPath
			out, runErr := cmd.CombinedOutput()
			cleanupSpikeBuildArtifacts(projectPath)
			if runErr != nil {
				msg := strings.TrimSpace(string(out))
				if len(msg) > 400 {
					msg = msg[:400]
				}
				return fmt.Errorf("%s failed: %w (%s)", strings.Join(buildCmd, " "), runErr, msg)
			}
		}
	}

	return nil
}

func cleanupSpikeBuildArtifacts(projectPath string) {
	for _, dir := range []string{"bin", "obj", "_build", "target", "__pycache__"} {
		_ = os.RemoveAll(filepath.Join(projectPath, dir))
	}
}

func commitSpikeToGit(projectPath string, writtenRelPaths []string) {
	args := append([]string{"add", "--"}, writtenRelPaths...)
	if _, err := os.Stat(filepath.Join(projectPath, "Makefile")); err == nil {
		args = append(args, "Makefile")
	}
	cmdAdd := exec.Command("git", args...)
	cmdAdd.Dir = projectPath
	_ = cmdAdd.Run()

	cmdCommit := exec.Command("git", "commit", "-m", "feat: initial spike solution walking skeleton")
	cmdCommit.Dir = projectPath
	_ = cmdCommit.Run()
}
