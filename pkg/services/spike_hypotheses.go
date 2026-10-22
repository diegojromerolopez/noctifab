package services

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

// SpikeCandidate represents one generated walking skeleton hypothesis.
type SpikeCandidate struct {
	Index   int
	Files   map[string]string
	Valid   bool
	FileCnt int
	Err     error
}

// GenerateSpikeHypotheses dispatches N parallel spike generation requests to explore
// multiple implementation hypotheses simultaneously.
func GenerateSpikeHypotheses(
	ctx context.Context,
	promptBody string,
	llmClient domain.LLMClient,
	hypCfg config.HypothesesConfig,
	timeoutSec int,
) []SpikeCandidate {
	count := hypCfg.GetAgents()
	if count <= 1 {
		count = 2
	}

	fmt.Printf("🚀 [Spike Multi-Hypothesis] Generating %d parallel walking skeleton hypotheses (Resolution: %s)...\n",
		count, hypCfg.GetResolution())

	var wg sync.WaitGroup
	candidates := make([]SpikeCandidate, count)

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			callCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
			defer cancel()

			resp, err := llmClient.Complete(callCtx, promptBody)
			if err != nil {
				candidates[idx] = SpikeCandidate{Index: idx + 1, Err: err}
				return
			}

			files := extractFilesFromSpikeResponse(resp)
			candidates[idx] = SpikeCandidate{
				Index:   idx + 1,
				Files:   files,
				Valid:   len(files) > 0,
				FileCnt: len(files),
			}
		}(i)
	}

	wg.Wait()
	return candidates
}

// ResolveSpikeHypotheses decides the final file set from multiple candidates based on resolution strategy.
func ResolveSpikeHypotheses(candidates []SpikeCandidate, resolution config.HypothesesResolution) map[string]string {
	if len(candidates) == 0 {
		return nil
	}

	// 1. If resolution is mixture, union the best complementary non-conflicting files
	if resolution == config.ResolutionMixture && len(candidates) >= 2 {
		mixture := make(map[string]string)
		// Seed with candidate having most files
		bestIdx := 0
		maxFiles := -1
		for i, c := range candidates {
			if c.FileCnt > maxFiles {
				maxFiles = c.FileCnt
				bestIdx = i
			}
		}

		for k, v := range candidates[bestIdx].Files {
			mixture[k] = v
		}

		// Union complementary missing files (e.g. tests or docs) from other candidates
		for i, c := range candidates {
			if i == bestIdx || !c.Valid {
				continue
			}
			for path, content := range c.Files {
				if _, exists := mixture[path]; !exists {
					mixture[path] = content
				}
			}
		}

		fmt.Printf("🧬 [Spike Multi-Hypothesis Mixture] Synthesized union of %d files across %d hypotheses.\n",
			len(mixture), len(candidates))
		return mixture
	}

	// 2. Default: "correct" / best candidate with most files and valid Makefile
	best := candidates[0]
	for _, c := range candidates {
		if !c.Valid {
			continue
		}
		if c.FileCnt > best.FileCnt {
			best = c
		}
	}

	fmt.Printf("🏆 [Spike Multi-Hypothesis Selection] Selected Hypothesis %d (%d files).\n", best.Index, best.FileCnt)
	return best.Files
}

func extractFilesFromSpikeResponse(resp *domain.LLMResponse) map[string]string {
	files := make(map[string]string)
	if resp == nil {
		return files
	}

	for _, act := range resp.Actions {
		switch act.Tool {
		case "write_files":
			if filesMap, ok := act.Args["files"].(map[string]any); ok {
				for p, c := range filesMap {
					if cStr, ok := c.(string); ok && strings.TrimSpace(p) != "" {
						files[filepath.Clean(strings.TrimSpace(p))] = cStr
					}
				}
			} else if filesMapStr, ok := act.Args["files"].(map[string]string); ok {
				for p, c := range filesMapStr {
					if strings.TrimSpace(p) != "" {
						files[filepath.Clean(strings.TrimSpace(p))] = c
					}
				}
			}
		case "write_file":
			p, _ := act.Args["path"].(string)
			c, _ := act.Args["content"].(string)
			if strings.TrimSpace(p) != "" {
				files[filepath.Clean(strings.TrimSpace(p))] = c
			}
		}
	}

	return files
}
