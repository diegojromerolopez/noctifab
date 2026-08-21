package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/notifier"
)

// DaemonStatus represents the operational standby state.
type DaemonStatus string

const (
	DaemonStatusIdle      DaemonStatus = "IDLE"
	DaemonStatusExecuting DaemonStatus = "EXECUTING"
	DaemonStatusPaused    DaemonStatus = "PAUSED"
)

// StoryExecutorFunc defines the signature for executing a single user story file.
type StoryExecutorFunc func(ctx context.Context, storyFile string) error

// StandbyEngineConfig holds dependencies and tuning parameters for the standby loop.
type StandbyEngineConfig struct {
	Repo     domain.StateRepository
	Mailbox  *CommandMailbox
	Notifier notifier.DesktopNotifier
	Executor StoryExecutorFunc
	BaseDir  string
	WatchFS  bool
	QueueCap int
}

// StandbyEngine coordinates the persistent, zero-idle-CPU background dark factory loop.
type StandbyEngine struct {
	repo     domain.StateRepository
	mailbox  *CommandMailbox
	notifier notifier.DesktopNotifier
	executor StoryExecutorFunc
	baseDir  string
	watchFS  bool
	storyCh  chan StoryWorkItem
	mu       sync.Mutex
	status   DaemonStatus
	current  string
}

// NewStandbyEngine creates a new StandbyEngine.
func NewStandbyEngine(cfg StandbyEngineConfig) *StandbyEngine {
	qCap := cfg.QueueCap
	if qCap <= 0 {
		qCap = 50
	}
	notif := cfg.Notifier
	if notif == nil {
		notif = &notifier.NoopNotifier{}
	}
	baseDir := cfg.BaseDir
	if baseDir == "" {
		baseDir = "."
	}

	return &StandbyEngine{
		repo:     cfg.Repo,
		mailbox:  cfg.Mailbox,
		notifier: notif,
		executor: cfg.Executor,
		baseDir:  baseDir,
		watchFS:  cfg.WatchFS,
		storyCh:  make(chan StoryWorkItem, qCap),
		status:   DaemonStatusIdle,
	}
}

// StoryChannel returns the write-end of the work item channel for enqueuing prompt orders.
func (se *StandbyEngine) StoryChannel() chan<- StoryWorkItem {
	return se.storyCh
}

// Enqueue sends a story work item into the execution queue.
func (se *StandbyEngine) Enqueue(item StoryWorkItem) bool {
	select {
	case se.storyCh <- item:
		return true
	default:
		return false
	}
}

// Status returns the current operational status of the standby engine.
func (se *StandbyEngine) Status() DaemonStatus {
	se.mu.Lock()
	defer se.mu.Unlock()
	return se.status
}

// Run executes the perpetual event loop. It blocks until ctx is cancelled.
func (se *StandbyEngine) Run(ctx context.Context) error {
	se.setStatus(DaemonStatusIdle, "")
	se.syncIdleState(ctx, "Ready for prompt orders")
	se.reconcilePendingOrders(ctx)

	if se.watchFS {
		watcher := NewFSWatcher(FSWatcherConfig{
			BaseDir: se.baseDir,
			OnStory: func(storyPath string) {
				se.Enqueue(StoryWorkItem{Path: storyPath})
			},
		})
		watcher.Start(ctx)
		defer watcher.Stop()
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case item, ok := <-se.storyCh:
			if !ok {
				return nil
			}
			se.processStoryWorkItem(ctx, item)
		}
	}
}

func (se *StandbyEngine) reconcilePendingOrders(ctx context.Context) {
	if se.repo == nil {
		return
	}
	state, err := se.repo.Load(ctx)
	if err != nil || state == nil || len(state.Orders) == 0 {
		return
	}
	for i := range state.Orders {
		if state.Orders[i].Status == "PENDING" || state.Orders[i].Status == "RUNNING" {
			if state.Orders[i].StoryPath != "" {
				if _, statErr := os.Stat(state.Orders[i].StoryPath); statErr == nil {
					se.Enqueue(StoryWorkItem{Path: state.Orders[i].StoryPath})
				} else {
					_ = saveStateWithBackoff(ctx, se.repo, func(s *domain.State) {
						for j := range s.Orders {
							if s.Orders[j].StoryPath == state.Orders[i].StoryPath {
								s.Orders[j].Status = "FAILED"
								s.Orders[j].UpdatedAt = time.Now().UTC()
							}
						}
					})
				}
			}
		}
	}
}

func (se *StandbyEngine) updateOrderStatus(ctx context.Context, storyPath string, status string) {
	if se.repo == nil {
		return
	}
	_ = saveStateWithBackoff(ctx, se.repo, func(state *domain.State) {
		for i := range state.Orders {
			if state.Orders[i].StoryPath == storyPath || filepath.Base(state.Orders[i].StoryPath) == filepath.Base(storyPath) {
				state.Orders[i].Status = status
				state.Orders[i].UpdatedAt = time.Now().UTC()
			}
		}
	})
}

func (se *StandbyEngine) processStoryWorkItem(ctx context.Context, item StoryWorkItem) {
	storyName := filepath.Base(item.Path)
	se.setStatus(DaemonStatusExecuting, storyName)
	se.updateOrderStatus(ctx, item.Path, "RUNNING")

	fmt.Printf("\n🚀 [Standby Daemon] Waking up for new story order: %s\n", storyName)

	var runErr error
	if se.executor != nil {
		runErr = se.executor(ctx, item.Path)
	}

	if runErr != nil {
		fmt.Printf("❌ [Standby Daemon] Story execution finished with error: %v\n", runErr)
		se.updateOrderStatus(ctx, item.Path, "FAILED")
		_ = se.notifier.Notify(ctx, notifier.NotifyBuildFailed, "Noctifab Build Failed",
			fmt.Sprintf("Story %s failed: %v", storyName, runErr))
	} else {
		fmt.Printf("✨ [Standby Daemon] Story %s completed successfully with 100%% test consensus!\n", storyName)
		se.updateOrderStatus(ctx, item.Path, "COMPLETED")
		_ = se.notifier.Notify(ctx, notifier.NotifyStoryCompleted, "Noctifab Dark Factory",
			fmt.Sprintf("Feature %s completed with 100%% green test consensus!", storyName))
	}

	se.setStatus(DaemonStatusIdle, "")
	se.syncIdleState(ctx, "Story completed — Ready for next order")
}

func (se *StandbyEngine) setStatus(st DaemonStatus, currentStory string) {
	se.mu.Lock()
	defer se.mu.Unlock()
	se.status = st
	se.current = currentStory
}

func (se *StandbyEngine) syncIdleState(ctx context.Context, msg string) {
	if se.repo == nil {
		return
	}
	state, err := se.repo.Load(ctx)
	if err != nil || state == nil {
		return
	}
	if state.StoryStatus == domain.StoryRunning || state.StoryStatus == "" {
		state.StoryStatus = domain.StoryIdle
		state.LastActions = append(state.LastActions, domain.Action{
			Tool:      "STANDBY",
			Reasoning: msg,
			Result:    "Daemon listening for orders",
			Success:   true,
			Timestamp: time.Now().UTC(),
		})
		_ = se.repo.Save(ctx, state)
	}
}
