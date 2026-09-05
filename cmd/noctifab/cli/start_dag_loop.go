package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

// StoryLoopOptions holds all parameters required to execute user story iteration loops.
type StoryLoopOptions struct {
	Cfg               *config.Config
	TargetDir         string
	StoryFiles        []string
	Repo              domain.StateRepository
	ExecutionReporter domain.ExecutionReporter
	ExecuteStory      func(ctx context.Context, currentStoryFile string) error
	GitClient         *services.GitClient
	TotalLoops        int
	ResumeRequested   bool
	WebEnabled        bool
	WebHost           string
	WebPort           int
}

// runStoryIterationLoops executes the user stories across iteration loops either concurrently (via StoryDAGScheduler) or sequentially.
func runStoryIterationLoops(ctx context.Context, opts StoryLoopOptions) (map[string]error, error) {
	storyOutcomes := make(map[string]error)
	for _, sf := range opts.StoryFiles {
		storyOutcomes[sf] = errors.New("pending")
	}

	var prevGitHead string
	var prevFailureSig string

	storyConcurrency := opts.Cfg.Agents.Orchestrator.Number
	if storyConcurrency <= 0 {
		if opts.Cfg.VCS.UseWorktrees {
			storyConcurrency = 2
		} else {
			storyConcurrency = 1
		}
	}

	for loopIdx := 1; loopIdx <= opts.TotalLoops; loopIdx++ {
		loopStart := time.Now().UTC()
		loopAttempted := 0
		loopSucceeded := 0

		if opts.TotalLoops > 1 {
			fmt.Printf("\n🔁 [Loop %d/%d] Executing Noctifab iteration loop (concurrency: %d)...\n", loopIdx, opts.TotalLoops, storyConcurrency)
		}

		if storyConcurrency > 1 && len(opts.StoryFiles) > 1 {
			// Story-Level Parallel Execution via StoryDAGScheduler with Cross-Story Task Pipelining
			dagScheduler := services.NewStoryDAGScheduler(storyConcurrency)
			dagScheduler.SetPipelined(true)
			for _, sf := range opts.StoryFiles {
				specBytes, _ := os.ReadFile(sf)
				dagScheduler.AddStory(services.StoryWorkItem{
					Path: sf,
					Spec: string(specBytes),
				})
			}

			// Pre-mark already completed stories from prior loops or resume
			for idx, currentStoryFile := range opts.StoryFiles {
				storyID := fmt.Sprintf("story-%04d", idx+1)
				featName := strings.TrimSuffix(filepath.Base(currentStoryFile), filepath.Ext(currentStoryFile))
				storyTitle := extractStoryTitle(currentStoryFile)

				if (loopIdx > 1 || opts.ResumeRequested) && isStoryCompletedSuccessfully(ctx, opts.Repo, currentStoryFile, storyID, featName) {
					fmt.Printf("ℹ [Loop %d] Verified story %s (%s) is completed successfully (all tasks passed) — skipping\n", loopIdx, storyID, storyTitle)
					storyOutcomes[currentStoryFile] = nil
					dagScheduler.MarkStoryCompleted(currentStoryFile)
				}
			}

			var outcomesMu sync.Mutex
			_ = dagScheduler.Execute(ctx, func(storyCtx context.Context, item services.StoryWorkItem) error {
				currentStoryFile := item.Path
				var idx int
				for i, f := range opts.StoryFiles {
					if f == currentStoryFile {
						idx = i
						break
					}
				}
				storyID := fmt.Sprintf("story-%04d", idx+1)
				featName := strings.TrimSuffix(filepath.Base(currentStoryFile), filepath.Ext(currentStoryFile))
				storyTitle := extractStoryTitle(currentStoryFile)
				storyMeta := domain.StoryMetadata{
					StoryID:     storyID,
					Source:      currentStoryFile,
					FeatureName: filepath.Base(currentStoryFile),
					Title:       storyTitle,
					Sequence:    idx + 1,
					StartedAt:   time.Now().UTC(),
				}

				outcomesMu.Lock()
				loopAttempted++
				outcomesMu.Unlock()

				if opts.ExecutionReporter != nil {
					opts.ExecutionReporter.BeginStory(storyCtx, storyMeta)
				}

				if opts.WebEnabled {
					fmt.Printf("\n🚀 Executing %s (%s)\n➜  Web Dashboard: http://%s:%d\n\n", storyID, storyTitle, opts.WebHost, opts.WebPort)
				} else {
					fmt.Printf("\n🚀 Executing %s (%s)\n\n", storyID, storyTitle)
				}

				storyErr := opts.ExecuteStory(storyCtx, currentStoryFile)

				outcomesMu.Lock()
				defer outcomesMu.Unlock()

				if storyErr != nil {
					storyOutcomes[currentStoryFile] = storyErr
					if opts.ExecutionReporter != nil {
						opts.ExecutionReporter.EndStory(storyCtx, storyID, domain.ExecutionFailed)
					}
					if opts.Repo != nil {
						if st, err := opts.Repo.Load(storyCtx); err == nil && st != nil {
							now := time.Now().UTC()
							for i, s := range st.Stories {
								if s.ID == featName || s.ID == storyID || s.FilePath == currentStoryFile {
									st.Stories[i].Status = domain.StoryFailed
									st.Stories[i].CompletedAt = &now
									st.Stories[i].UpdatedAt = now
									break
								}
							}
							_ = opts.Repo.Save(storyCtx, st)
						}
					}
					fmt.Printf("⚠️ Story %s (%s) encountered failure in Loop %d: %v (continuing loop pass)\n", storyID, storyTitle, loopIdx, storyErr)
					return storyErr
				}

				loopSucceeded++
				storyOutcomes[currentStoryFile] = nil
				if opts.ExecutionReporter != nil {
					opts.ExecutionReporter.EndStory(storyCtx, storyID, domain.ExecutionSuccess)
				}
				if opts.Repo != nil {
					if st, err := opts.Repo.Load(storyCtx); err == nil && st != nil {
						now := time.Now().UTC()
						for i, s := range st.Stories {
							if s.ID == featName || s.ID == storyID || s.FilePath == currentStoryFile {
								st.Stories[i].Status = domain.StorySuccess
								st.Stories[i].CompletedAt = &now
								st.Stories[i].UpdatedAt = now
								break
							}
						}
						_ = opts.Repo.Save(storyCtx, st)
					}
				}
				return nil
			})
		} else {
			// Sequential Execution Loop
			for idx, currentStoryFile := range opts.StoryFiles {
				storyID := fmt.Sprintf("story-%04d", idx+1)
				featName := strings.TrimSuffix(filepath.Base(currentStoryFile), filepath.Ext(currentStoryFile))
				storyTitle := extractStoryTitle(currentStoryFile)
				storyMeta := domain.StoryMetadata{
					StoryID:     storyID,
					Source:      currentStoryFile,
					FeatureName: filepath.Base(currentStoryFile),
					Title:       storyTitle,
					Sequence:    idx + 1,
					StartedAt:   time.Now().UTC(),
				}

				if (loopIdx > 1 || opts.ResumeRequested) && isStoryCompletedSuccessfully(ctx, opts.Repo, currentStoryFile, storyID, featName) {
					fmt.Printf("ℹ [Loop %d] Verified story %s (%s) is completed successfully (all tasks passed) — skipping\n", loopIdx, storyID, storyTitle)
					storyOutcomes[currentStoryFile] = nil
					continue
				}

				loopAttempted++
				if opts.ExecutionReporter != nil {
					opts.ExecutionReporter.BeginStory(ctx, storyMeta)
				}

				if opts.WebEnabled {
					fmt.Printf("\n🚀 Executing %s (%s)\n➜  Web Dashboard: http://%s:%d\n\n", storyID, storyTitle, opts.WebHost, opts.WebPort)
				} else {
					fmt.Printf("\n🚀 Executing %s (%s)\n\n", storyID, storyTitle)
				}

				storyErr := opts.ExecuteStory(ctx, currentStoryFile)
				if storyErr != nil {
					storyOutcomes[currentStoryFile] = storyErr
					if opts.ExecutionReporter != nil {
						opts.ExecutionReporter.EndStory(ctx, storyID, domain.ExecutionFailed)
					}
					if opts.Repo != nil {
						if st, err := opts.Repo.Load(ctx); err == nil && st != nil {
							now := time.Now().UTC()
							for i, s := range st.Stories {
								if s.ID == featName || s.ID == storyID || s.FilePath == currentStoryFile {
									st.Stories[i].Status = domain.StoryFailed
									st.Stories[i].CompletedAt = &now
									st.Stories[i].UpdatedAt = now
									break
								}
							}
							_ = opts.Repo.Save(ctx, st)
						}
					}
					fmt.Printf("⚠️ Story %s (%s) encountered failure in Loop %d: %v (continuing loop pass)\n", storyID, storyTitle, loopIdx, storyErr)
				} else {
					loopSucceeded++
					storyOutcomes[currentStoryFile] = nil
					if opts.ExecutionReporter != nil {
						opts.ExecutionReporter.EndStory(ctx, storyID, domain.ExecutionSuccess)
					}
					if opts.Repo != nil {
						if st, err := opts.Repo.Load(ctx); err == nil && st != nil {
							now := time.Now().UTC()
							for i, s := range st.Stories {
								if s.ID == featName || s.ID == storyID || s.FilePath == currentStoryFile {
									st.Stories[i].Status = domain.StorySuccess
									st.Stories[i].CompletedAt = &now
									st.Stories[i].UpdatedAt = now
									break
								}
							}
							_ = opts.Repo.Save(ctx, st)
						}
					}
				}
			}
		}

		allSucceeded := true
		for _, sf := range opts.StoryFiles {
			if storyOutcomes[sf] != nil {
				allSucceeded = false
				break
			}
		}

		loopDurationMS := time.Since(loopStart).Milliseconds()
		loopOutcome := domain.ExecutionSuccess
		if !allSucceeded {
			loopOutcome = domain.ExecutionFailed
		}

		if opts.TotalLoops > 1 {
			fmt.Printf("\n📊 [Loop %d/%d Summary] Attempted: %d | Succeeded: %d | Duration: %v | Outcome: %s\n",
				loopIdx, opts.TotalLoops, loopAttempted, loopSucceeded, time.Duration(loopDurationMS)*time.Millisecond, loopOutcome)
		}

		if allSucceeded {
			if opts.TotalLoops > 1 && loopIdx < opts.TotalLoops {
				fmt.Printf("\n✨ All %d user stories completed successfully in Loop %d. Completing run.\n", len(opts.StoryFiles), loopIdx)
			}
			break
		}

		// Loop Stagnation & Deadlock Circuit Breaker
		currentGitHead, _ := opts.GitClient.Run(ctx, false, "rev-parse", "HEAD")
		currentFailureSig := computeFailureSignature(storyOutcomes)
		if loopIdx > 1 && currentGitHead == prevGitHead && currentFailureSig == prevFailureSig {
			fmt.Printf("\n⚠️ [Stagnation Circuit Breaker] Loop %d produced zero codebase changes with identical failure signatures as Loop %d. Terminating loop iteration to prevent token exhaustion.\n", loopIdx, loopIdx-1)
			break
		}
		prevGitHead = currentGitHead
		prevFailureSig = currentFailureSig
	}

	return storyOutcomes, nil
}
