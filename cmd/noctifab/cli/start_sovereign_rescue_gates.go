package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

func verifySovereignE2E(ctx context.Context, targetDir string, runner services.Sandbox, cfg *config.Config) (bool, string) {
	if runner == nil {
		return true, ""
	}
	e2eMode := ""
	e2eCmd := ""
	if cfg != nil {
		e2eMode = cfg.Sandbox.GetE2EMode()
		e2eCmd = cfg.Sandbox.GetE2ECommand()
	}
	detectedCmd := services.DetectE2ECommand(targetDir, e2eMode, e2eCmd)
	if detectedCmd == "" {
		return true, ""
	}

	_ = services.PrepareTestEnvironment(targetDir)

	fmt.Printf("🔍 [Sovereign Rescue] Running E2E verification gate: %q...\n", detectedCmd)
	e2eOut, e2eErr := runner.RunCommand(ctx, targetDir, detectedCmd, "")
	if e2eErr != nil {
		out := e2eOut
		if len(out) > 2000 {
			out = out[:2000] + "\n... [truncated]"
		}
		return false, fmt.Sprintf("❌ E2E test execution FAILED (%s):\n%s\nError: %v", detectedCmd, out, e2eErr)
	}
	fmt.Printf("✅ [Sovereign Rescue] E2E verification gate PASSED (%s)!\n", detectedCmd)
	return true, ""
}

func auditSovereignAntiStub(targetDir string) (bool, string) {
	antiStub := services.NewAntiStubValidator()
	violations, _ := antiStub.ValidateWorkspace(targetDir, nil)
	if len(violations) == 0 {
		return true, ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "⚠️ Anti-Stub & Non-Tautological Quality Gate FAILED (%d violation(s)):\n", len(violations))
	sb.WriteString("Tests and recipes must not be tautological, vacuous, or mask errors with shell tricks. You MUST write genuine behavioral assertions:\n")
	for _, v := range violations {
		fmt.Fprintf(&sb, "- %s:%d: [%s] %s\n", v.Path, v.Line, v.Rule, v.Snippet)
	}
	return false, sb.String()
}

// captureSovereignBaselineCommit captures the current git HEAD commit hash if available.
func captureSovereignBaselineCommit(ctx context.Context, gitClient *services.GitClient) string {
	if gitClient == nil {
		return ""
	}
	out, err := gitClient.Run(ctx, true, "rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// rollbackSovereignTurn resets the working tree back to targetRef to revert broken mutations.
func rollbackSovereignTurn(ctx context.Context, gitClient *services.GitClient, targetRef string) error {
	if gitClient == nil || targetRef == "" {
		return nil
	}
	_, err := gitClient.Run(ctx, true, "reset", "--hard", targetRef)
	return err
}

func updateRescueSuccessState(ctx context.Context, repo domain.StateRepository, storyFiles, failedStories, acceptanceGaps []string) error {
	st, err := repo.Load(ctx)
	if err != nil || st == nil {
		return err
	}

	st.BuildStatus = domain.BuildPassing
	st.StoryStatus = domain.StorySuccess
	st.StoryError = ""

	isWholeProjectAudit := len(acceptanceGaps) > 0
	targetStoryKeys := make(map[string]bool)
	for _, fs := range failedStories {
		parts := strings.Fields(fs)
		if len(parts) > 0 {
			clean := strings.TrimSuffix(parts[0], filepath.Ext(parts[0]))
			targetStoryKeys[clean] = true
			if idx := strings.Index(clean, "-"); idx > 0 {
				if secondIdx := strings.Index(clean[idx+1:], "-"); secondIdx > 0 {
					targetStoryKeys[clean[:idx+1+secondIdx]] = true
				}
			}
		}
	}

	matchesStory := func(story domain.Story) bool {
		if isWholeProjectAudit || len(targetStoryKeys) == 0 {
			return true
		}
		if targetStoryKeys[story.ID] || targetStoryKeys[story.Title] {
			return true
		}
		for key := range targetStoryKeys {
			if strings.Contains(story.ID, key) || strings.Contains(story.Title, key) || strings.Contains(story.FilePath, key) {
				return true
			}
		}
		return false
	}

	matchedStories := 0
	matchedStoryIDs := make(map[string]bool)
	for i := range st.Stories {
		if matchesStory(st.Stories[i]) {
			st.Stories[i].Status = domain.StorySuccess
			matchedStories++
			matchedStoryIDs[st.Stories[i].ID] = true
			matchedStoryIDs[st.Stories[i].Title] = true
		}
	}

	existingStoryIDs := make(map[string]bool)
	for _, s := range st.Stories {
		existingStoryIDs[s.ID] = true
	}

	for idx, sf := range storyFiles {
		storyID := fmt.Sprintf("story-%04d", idx+1)
		featName := strings.TrimSuffix(filepath.Base(sf), filepath.Ext(sf))
		tempStory := domain.Story{ID: storyID, Title: featName, FilePath: sf}
		if matchesStory(tempStory) && !existingStoryIDs[storyID] {
			st.Stories = append(st.Stories, domain.Story{
				ID:     storyID,
				Title:  featName,
				Status: domain.StorySuccess,
			})
			matchedStoryIDs[storyID] = true
			matchedStoryIDs[featName] = true
		}
	}

	allStoriesMatched := len(st.Stories) > 0 && matchedStories == len(st.Stories)
	for i := range st.Tasks {
		task := &st.Tasks[i]
		belongs := isWholeProjectAudit || len(targetStoryKeys) == 0 || allStoriesMatched
		if !belongs {
			if matchedStoryIDs[task.StoryID] {
				belongs = true
			} else {
				for key := range targetStoryKeys {
					if strings.Contains(task.ID, key) || strings.Contains(task.StoryID, key) {
						belongs = true
						break
					}
				}
			}
		}
		if belongs {
			task.Status = domain.TaskSuccess
		}
	}

	action := domain.Action{
		Timestamp: time.Now().UTC(),
		Tool:      "sovereign_rescue_success",
		Reasoning: "Autonomous sovereign rescue completed requirements and passed validation gates",
		Success:   true,
	}
	st.LastActions = append(st.LastActions, action)

	return repo.Save(ctx, st)
}
