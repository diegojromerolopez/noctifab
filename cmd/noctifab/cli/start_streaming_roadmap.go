package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

// GenerateRoadmapStreaming starts Stage 1 & Stage 2 roadmap generation and streams each story
// to the dagScheduler as soon as it is expanded in Stage 2, unblocking immediate story execution.
func GenerateRoadmapStreaming(
	ctx context.Context,
	targetDir string,
	cfg *config.Config,
	llmClient domain.LLMClient,
	renderer services.PromptRenderer,
	reporter domain.ExecutionReporter,
	dagScheduler *services.StoryDAGScheduler,
) <-chan error {
	doneCh := make(chan error, 1)

	go func() {
		defer close(doneCh)
		if reporter != nil {
			reporter.Observe(ctx, domain.ExecutionEvent{
				Kind: domain.EventPhaseStarted,
				Name: "roadmap_generation",
				At:   time.Now().UTC(),
			})
			defer reporter.Observe(ctx, domain.ExecutionEvent{
				Kind: domain.EventPhaseFinished,
				Name: "roadmap_generation",
				At:   time.Now().UTC(),
			})
		}

		pmCfg := cfg.Agents.ProductManager
		pmCtx := services.WithCompactionMode(ctx, cfg.Context.GetCompactionMode())
		pmCtx = domain.WithAuditMode(pmCtx, pmCfg.AuditMode)

		onStoryReady := func(filePath, content string) {
			fmt.Printf("🚀 [Streaming Roadmap Handoff] Story %s expanded! Ingesting into execution scheduler...\n", filepath.Base(filePath))
			if dagScheduler != nil {
				dagScheduler.AddStory(services.StoryWorkItem{
					Path: filePath,
					Spec: content,
				})
			}
		}

		genErr := services.GenerateRoadmapWithStreaming(
			pmCtx,
			targetDir,
			llmClient,
			renderer,
			pmCfg.Passes,
			pmCfg.GetMaxUserStories(),
			pmCfg.GetMinComplexity(),
			pmCfg.GetMaxComplexity(),
			onStoryReady,
		)

		if dagScheduler != nil {
			dagScheduler.CloseStoryStream()
		}

		if genErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Product Manager Agent story generation encountered error: %v\n", genErr)
		}
		doneCh <- genErr
	}()

	return doneCh
}
