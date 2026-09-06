package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

// SpeculativePlanner proactively decomposes downstream user stories into task DAGs
// in the background while the active story executes its tasks. By overlapping the
// 10-25s LLM planning latency, downstream stories begin executing with 0ms startup delay.
type SpeculativePlanner struct {
	planFunc func(ctx context.Context, storyFile string) error
	repo     domain.StateRepository
	planned  map[string]bool
	mu       sync.Mutex
}

// NewSpeculativePlanner constructs a new SpeculativePlanner.
func NewSpeculativePlanner(repo domain.StateRepository, planFunc func(ctx context.Context, storyFile string) error) *SpeculativePlanner {
	return &SpeculativePlanner{
		planFunc: planFunc,
		repo:     repo,
		planned:  make(map[string]bool),
	}
}

// PrePlanQueuedStories scans the given storyFiles list and asynchronously triggers
// task DAG decomposition for any upcoming story that hasn't already been planned.
func (p *SpeculativePlanner) PrePlanQueuedStories(ctx context.Context, storyFiles []string, currentStory string) {
	if p == nil || p.planFunc == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, sf := range storyFiles {
		if sf == currentStory || p.planned[sf] {
			continue
		}
		storyID := services.ExtractStoryID(sf)
		featName := filepath.Base(sf)

		// Check if state repository already has tasks planned for this story.
		if p.repo != nil {
			if st, err := p.repo.Load(ctx); err == nil && st != nil {
				alreadyPlanned := false
				for _, t := range st.Tasks {
					if (storyID != "" && t.StoryID == storyID) || (featName != "" && t.StoryID == featName) {
						alreadyPlanned = true
						break
					}
				}
				if alreadyPlanned {
					p.planned[sf] = true
					continue
				}
			}
		}

		p.planned[sf] = true
		go func(file string, sID string) {
			planCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
			defer cancel()
			fmt.Printf("⚡ [Speculative Macro-Planning] Overlapping decomposition for %s (%s) in background...\n", sID, filepath.Base(file))
			if err := p.planFunc(planCtx, file); err != nil {
				fmt.Printf("ℹ [Speculative Macro-Planning] Pre-planning for %s deferred to execution phase: %v\n", sID, err)
			} else {
				fmt.Printf("✨ [Speculative Macro-Planning] Story %s successfully pre-planned in background (0ms startup latency).\n", sID)
			}
		}(sf, storyID)
	}
}

// MarkPlanned marks a story file as already planned.
func (p *SpeculativePlanner) MarkPlanned(storyFile string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.planned[storyFile] = true
}

// IsPlanned checks whether a story file has already been planned.
func (p *SpeculativePlanner) IsPlanned(storyFile string) bool {
	if p == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.planned[storyFile]
}
