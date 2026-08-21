package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
)

// SpecOrchestrator coordinates multi-model spec drafting, interactive HITL feedback loops, and consensus auditing.
type SpecOrchestrator struct {
	cfg        *config.Config
	router     *llm.ResilientLLMRouter
	pipeline   *SpecMultiAgentPipeline
	auditor    *SpecConsensusAuditor
	intentEval *SpecIntentDetector
	renderer   *SpecRenderer
}

// NewSpecOrchestrator instantiates a SpecOrchestrator.
func NewSpecOrchestrator(cfg *config.Config, router *llm.ResilientLLMRouter, renderer PromptRenderer, specRenderer *SpecRenderer) *SpecOrchestrator {
	if renderer == nil {
		renderer = prompts.NewDefaultRenderer()
	}
	if specRenderer == nil {
		specRenderer = NewSpecRenderer()
	}

	var leadClient domain.LLMClient
	if router != nil {
		candidates := router.ResolveCandidatesForRole("product_manager")
		if len(candidates) > 0 {
			leadClient = candidates[0].Client
		}
	}

	return &SpecOrchestrator{
		cfg:        cfg,
		router:     router,
		pipeline:   NewSpecMultiAgentPipeline(cfg, router, renderer),
		auditor:    NewSpecConsensusAuditor(cfg, router, renderer),
		intentEval: NewSpecIntentDetector(leadClient),
		renderer:   specRenderer,
	}
}

// RunSessionOptions encapsulates configuration parameters for the specification session.
type RunSessionOptions struct {
	ProjectPath    string
	InitialPrompt  string
	TargetFile     string
	NonInteractive bool
	EnableAudit    bool
}

// RunSession executes the specification creation or refinement session.
func (o *SpecOrchestrator) RunSession(ctx context.Context, opts RunSessionOptions) (*domain.SpecSession, error) {
	targetFile := opts.TargetFile
	if targetFile == "" {
		if o.cfg != nil {
			targetFile = o.cfg.Spec.GetOutputFile()
		} else {
			targetFile = "SPEC.md"
		}
	}
	if !filepath.IsAbs(targetFile) {
		targetFile = filepath.Join(opts.ProjectPath, targetFile)
	}

	session := &domain.SpecSession{
		ID:          fmt.Sprintf("spec-%d", time.Now().Unix()),
		ProjectPath: opts.ProjectPath,
		TargetFile:  targetFile,
		CreatedAt:   time.Now().UTC(),
	}

	snapshots := storage.NewSpecSnapshotManager(opts.ProjectPath)

	var existingSpec string
	if data, err := os.ReadFile(targetFile); err == nil {
		existingSpec = string(data)
	}

	o.renderer.PrintHeader("Noctifab Interactive Specification Generator")

	// 1. Initial Generation / Loading
	var currentDraft string
	if existingSpec != "" && strings.TrimSpace(opts.InitialPrompt) == "" {
		o.renderer.PrintInfo(fmt.Sprintf("Loaded existing specification from %s", targetFile))
		currentDraft = existingSpec
	} else {
		o.renderer.PrintInfo("Executing 4-stage multi-model drafting pipeline (PM ➔ Systems Architect ➔ Test Architect ➔ QA)...")
		draft, err := o.pipeline.ExecutePass(ctx, opts.InitialPrompt, existingSpec)
		if err != nil {
			return nil, fmt.Errorf("spec drafting failed: %w", err)
		}
		currentDraft = draft
	}

	// 2. Consensus Audit Pass
	enableAudit := opts.EnableAudit
	if o.cfg != nil && !o.cfg.Spec.IsConsensusEnabled() {
		enableAudit = false
	}
	if enableAudit {
		o.renderer.PrintInfo("Running multi-model consensus audit to eliminate single-provider bias & contradictions...")
		audited, err := o.auditor.AuditAndReconcile(ctx, currentDraft, opts.InitialPrompt)
		if err == nil && strings.TrimSpace(audited) != "" {
			currentDraft = audited
		}
	}

	rev1 := session.AddRevision(currentDraft, opts.InitialPrompt, domain.SpecTurnInitial, "", 0)
	if snapPath, sha, err := snapshots.SaveSnapshot(rev1.Version, rev1.Content, ""); err == nil {
		session.Revisions[0].SnapshotPath = snapPath
		session.Revisions[0].SHA256 = sha
	}

	// 3. Non-Interactive Mode
	if opts.NonInteractive {
		if err := o.saveSpecFile(session.TargetFile, currentDraft); err != nil {
			return session, err
		}
		o.renderer.PrintSuccess(fmt.Sprintf("Saved finalized specification to %s", session.TargetFile))
		session.IsComplete = true
		return session, nil
	}

	// 4. Interactive Review & Refinement Loop
	turn := 1
	for {
		o.renderer.RenderSpecPreview(session.CurrentSpec, turn)

		humanInput, err := o.renderer.PromptUserFeedback(turn)
		if err != nil {
			// User pressed Ctrl+C or EOF -> save progress and exit
			break
		}

		// Handle Time-Travel Commands (undo, redo, history, checkout)
		if isTT, op, targetVer := o.intentEval.DetectTimeTravelIntent(humanInput); isTT {
			switch op {
			case "history":
				o.renderer.RenderHistory(session.Revisions, session.ActiveVerIndex)
			case "undo":
				rev, uErr := session.Undo()
				if uErr != nil {
					o.renderer.PrintError(uErr.Error())
				} else {
					lineCount := len(strings.Split(rev.Content, "\n"))
					o.renderer.RenderRollback(rev.Version, lineCount)
				}
			case "redo":
				rev, rErr := session.Redo()
				if rErr != nil {
					o.renderer.PrintError(rErr.Error())
				} else {
					lineCount := len(strings.Split(rev.Content, "\n"))
					o.renderer.RenderRollback(rev.Version, lineCount)
				}
			case "checkout":
				rev, cErr := session.Checkout(targetVer)
				if cErr != nil {
					o.renderer.PrintError(cErr.Error())
				} else {
					lineCount := len(strings.Split(rev.Content, "\n"))
					o.renderer.RenderCheckout(rev.Version, lineCount)
				}
			}
			continue
		}

		// Check for termination / approval intent
		isStop, reasoning := o.intentEval.IsTerminationIntent(ctx, humanInput)
		if isStop {
			o.renderer.PrintApprovalMessage(humanInput, reasoning)
			break
		}

		turn++
		o.renderer.PrintInfo(fmt.Sprintf("[Turn %d] Refining specification with your feedback...", turn))

		newDraft, err := o.pipeline.ExecuteRefinePass(ctx, session.CurrentSpec, humanInput, session.Revisions)
		if err != nil {
			o.renderer.PrintError(fmt.Sprintf("Refinement error: %v. Retrying with current spec.", err))
			continue
		}

		if enableAudit {
			audited, err := o.auditor.AuditAndReconcile(ctx, newDraft, humanInput)
			if err == nil && strings.TrimSpace(audited) != "" {
				newDraft = audited
			}
		}

		diff := o.renderer.CalculateDiff(session.CurrentSpec, newDraft)
		o.renderer.RenderDiff(diff)

		rev := session.AddRevision(newDraft, humanInput, domain.SpecTurnRefine, diff, 0)
		if snapPath, sha, err := snapshots.SaveSnapshot(rev.Version, rev.Content, diff); err == nil {
			idx := len(session.Revisions) - 1
			session.Revisions[idx].SnapshotPath = snapPath
			session.Revisions[idx].SHA256 = sha
		}
	}

	// 5. Save Finalized SPEC.md
	if err := o.saveSpecFile(session.TargetFile, session.CurrentSpec); err != nil {
		return session, fmt.Errorf("failed to save specification file %s: %w", session.TargetFile, err)
	}
	o.renderer.PrintSuccess(fmt.Sprintf("Saved finalized specification to %s", session.TargetFile))
	session.IsComplete = true

	// 6. Offer to Bootstrap Roadmap
	shouldAutoRoadmap := true
	if o.cfg != nil {
		shouldAutoRoadmap = o.cfg.Spec.ShouldAutoGenerateRoadmap()
	}

	if shouldAutoRoadmap && o.renderer.PromptYesNo("Would you like to generate the initial user story roadmap now?", true) {
		pmCandidates := o.router.ResolveCandidatesForRole("product_manager")
		if len(pmCandidates) > 0 && pmCandidates[0].Client != nil {
			o.renderer.PrintInfo("Product Manager Agent generating user story roadmap...")
			if err := GenerateRoadmapWithPasses(ctx, opts.ProjectPath, pmCandidates[0].Client, nil, 2); err != nil {
				o.renderer.PrintError(fmt.Sprintf("Roadmap generation warning: %v", err))
			} else {
				o.renderer.PrintSuccess("Roadmap successfully generated under roadmap/user-stories/")
			}
		}
	}

	return session, nil
}

func (o *SpecOrchestrator) saveSpecFile(path string, content string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}
