package services

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/google/uuid"
)

// StoryWorkItem represents a single user story queued for processing in server mode.
type StoryWorkItem struct {
	// Path is the absolute path to the markdown user story file.
	Path string
	// Spec is the raw markdown text of the user story specification.
	Spec string
	// LogPath is the absolute path to the per-story log file.
	// Conventionally: .noctifab/logs/roadmap/<story-name>.log
	LogPath string
}

// StartUserStoryCmd queues a single user story for planning and execution.
// The orchestrator receives this command via the CommandMailbox and begins
// a full plan → execute → PR cycle for the given story file.
type StartUserStoryCmd struct {
	// Path is the absolute path to the user story markdown file.
	Path string
	// StoryCh is the channel through which the resolved StoryWorkItem is forwarded.
	StoryCh chan<- StoryWorkItem
}

// Execute reads the user story markdown file and sends the work item to the story channel.
func (c *StartUserStoryCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	absPath, err := resolveAbsPath(c.Path)
	if err != nil {
		return fmt.Errorf("cannot resolve story path %q: %w", c.Path, err)
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read user story %q: %w", absPath, err)
	}

	logPath, err := storyLogPath(absPath)
	if err != nil {
		return fmt.Errorf("cannot determine log path for story %q: %w", absPath, err)
	}

	select {
	case c.StoryCh <- StoryWorkItem{Path: absPath, Spec: string(data), LogPath: logPath}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// StartDirectoryCmd scans a directory for markdown user story files (sorted lexicographically)
// and enqueues each one as a StoryWorkItem in sequence.
type StartDirectoryCmd struct {
	// DirPath is the absolute or relative path to the user story directory.
	DirPath string
	// StoryCh is the channel through which StoryWorkItems are forwarded.
	StoryCh chan<- StoryWorkItem
}

// Execute walks the directory, finds all *.md files (sorted), reads each, and sends work items.
func (c *StartDirectoryCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	absDir, err := resolveAbsPath(c.DirPath)
	if err != nil {
		return fmt.Errorf("cannot resolve directory path %q: %w", c.DirPath, err)
	}

	var mdFiles []string
	walkErr := filepath.WalkDir(absDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(path), ".md") {
			mdFiles = append(mdFiles, path)
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("failed to scan directory %q: %w", absDir, walkErr)
	}

	sort.Strings(mdFiles)

	for _, mdPath := range mdFiles {
		data, err := os.ReadFile(mdPath)
		if err != nil {
			return fmt.Errorf("failed to read user story %q: %w", mdPath, err)
		}

		logPath, err := storyLogPath(mdPath)
		if err != nil {
			return fmt.Errorf("cannot determine log path for story %q: %w", mdPath, err)
		}

		select {
		case c.StoryCh <- StoryWorkItem{Path: mdPath, Spec: string(data), LogPath: logPath}:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// resolveAbsPath resolves a path that may contain environment variable references
// (e.g., $HOME) and converts it to an absolute path.
func resolveAbsPath(path string) (string, error) {
	expanded := os.ExpandEnv(path)
	if filepath.IsAbs(expanded) {
		return filepath.Clean(expanded), nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	return filepath.Clean(filepath.Join(cwd, expanded)), nil
}

// storyLogPath derives the per-story log file path from the absolute story file path.
// Log files are stored at <cwd>/.noctifab/logs/roadmap/<story-name>.log.
// The log directory is created if it does not already exist.
func storyLogPath(absStoryPath string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine working directory: %w", err)
	}
	logDir := filepath.Join(cwd, ".noctifab", "logs", "roadmap")
	if mkErr := os.MkdirAll(logDir, 0755); mkErr != nil {
		return "", fmt.Errorf("cannot create log directory %s: %w", logDir, mkErr)
	}
	base := filepath.Base(absStoryPath)
	logName := strings.TrimSuffix(base, filepath.Ext(base)) + ".log"
	return filepath.Join(logDir, logName), nil
}

// NewStateForStory creates a fresh domain.State for a new user story execution.
func NewStateForStory(projectPath, storyPath, baseBranch, branchPrefix string) *domain.State {
	slug := storySlug(storyPath)
	integrationBranch := branchPrefix + "story-" + slug
	if branchPrefix == "" {
		integrationBranch = "noctifab/story-" + slug
	}
	featName := filepath.Base(storyPath)
	return &domain.State{
		ID:          uuid.New().String(),
		ProjectPath: projectPath,
		Version:     0,
		BuildStatus: domain.BuildUnknown,
		Metadata: domain.StateMetadata{
			InputSource:       "markdown",
			InputPath:         storyPath,
			FeatureName:       featName,
			BaseBranch:        baseBranch,
			IntegrationBranch: integrationBranch,
			TotalCostUSD:      "0.00000",
		},
	}
}

// storySlug converts a file path to a URL/branch-safe slug.
func storySlug(path string) string {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	slug := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, base)
	return strings.ToLower(slug)
}

// MarkStoryInterruptedCmd marks all in-progress tasks as INTERRUPTED on graceful daemon shutdown.
type MarkStoryInterruptedCmd struct{}

// Execute sets all TaskInProgress tasks to TaskInterrupted and saves state.
func (c *MarkStoryInterruptedCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return err
	}

	modified := false
	for i := range state.Tasks {
		if state.Tasks[i].Status == domain.TaskInProgress {
			state.Tasks[i].Status = domain.TaskInterrupted
			state.Tasks[i].UpdatedAt = time.Now()
			modified = true
		}
	}

	if modified {
		return repo.Save(ctx, state)
	}
	return nil
}
