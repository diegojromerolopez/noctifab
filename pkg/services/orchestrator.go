package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/prompts"
)

// RepairHandler defines the contract for automatic repair of hung/failed test suites.
type RepairHandler interface {
	AttemptRepair(ctx context.Context, state *domain.State, task domain.Task, watchdogOutput string, watchdogErr error) (*RepairResult, error)
}

// PromptRenderer renders the effective prompt for an (agent, action) key.
// Implemented by *prompts.Renderer; injected so tests can substitute it.
type PromptRenderer interface {
	Render(agent, action string, data any) (prompts.Rendered, error)
}

type OrchestratorConfig struct {
	Architecture         string
	TaskExecutionOrder   string
	GeneratorsNumber     int
	GeneratorsIterations int
	TestersNumber        int
	TestersIterations    int
	PollInterval         time.Duration
	MaxRetries           int
	Concurrency          int
	UseWorktrees         bool
	OCCMaxRetries        int
	OCCBackoffBase       time.Duration
	OCCBackoffFactor     float64
	MaxDuration          time.Duration
	AutoCreatePR         bool
	CreateBranch         bool
	MaxActions           int
	ExcludePaths         []string
	MetricsEnabled       bool
	MetricsOutputPath    string
	Context              config.ContextConfig
	WorkspaceCache       config.WorkspaceCacheConfig
	QA                   config.QAConfig
}

// QADependencies contains the optional infrastructure used only when QA is enabled.
type QADependencies struct {
	WorkspaceFactory ReviewWorkspaceFactory
	ArtifactBuilder  QAArtifactBuilder
	Sandbox          QASandboxRunner
	FileSystem       QAFileSystem
	Clock            QAClock
}

func (c OrchestratorConfig) GetWorkspaceCache() config.WorkspaceCacheConfig {
	return c.WorkspaceCache
}

type Orchestrator struct {
	repo              domain.StateRepository
	registry          Registry
	llmClient         domain.LLMClient
	validator         Validator
	scheduler         *Scheduler
	git               *GitClient
	rebaseQueue       *RebaseQueue
	evaluator         *TestValidator
	vcsClient         domain.VCSClient
	cfg               OrchestratorConfig
	mailbox           *CommandMailbox
	watchdogRepair    RepairHandler
	promptRenderer    PromptRenderer
	metricsMu         sync.RWMutex
	metricsCollector  *MetricsCollector
	unblocker         *UnblockerAgent
	qa                *QARuntimeCoordinator
	timesMu           sync.Mutex
	storyStartedAt    time.Time
	totalActions      int64
	taskCompletedChan chan struct{}
	lastWorkspaceSync time.Time
	observer          domain.ExecutionObserver
	// executeTaskFn is the task execution entry point used by the dispatch
	// loop. It defaults to (*Orchestrator).executeTask and exists as an
	// injection seam for unit tests.
	executeTaskFn func(ctx context.Context, stateID, taskID string)
}

type OrchestratorRuntimeDependencies struct {
	Mailbox        *CommandMailbox
	WatchdogRepair RepairHandler
	PromptRenderer PromptRenderer
	QA             *QARuntimeCoordinator
	Observer       domain.ExecutionObserver
}

func NewOrchestratorWithRuntime(
	repo domain.StateRepository,
	reg Registry,
	client domain.LLMClient,
	val Validator,
	sched *Scheduler,
	git *GitClient,
	queue *RebaseQueue,
	eval *TestValidator,
	vcsClient domain.VCSClient,
	cfg OrchestratorConfig,
	runtime OrchestratorRuntimeDependencies,
) *Orchestrator {
	if runtime.PromptRenderer == nil {
		runtime.PromptRenderer = prompts.NewDefaultRenderer()
	}
	if runtime.Observer == nil {
		runtime.Observer = &NoopExecutionReporter{}
	}
	o := &Orchestrator{
		repo:              repo,
		registry:          reg,
		llmClient:         client,
		validator:         val,
		scheduler:         sched,
		git:               git,
		rebaseQueue:       queue,
		evaluator:         eval,
		vcsClient:         vcsClient,
		cfg:               cfg,
		mailbox:           runtime.Mailbox,
		watchdogRepair:    runtime.WatchdogRepair,
		promptRenderer:    runtime.PromptRenderer,
		qa:                runtime.QA,
		metricsCollector:  NewMetricsCollector(cfg.MetricsEnabled),
		taskCompletedChan: make(chan struct{}, 100),
		observer:          runtime.Observer,
	}
	o.executeTaskFn = o.executeTask
	if queue != nil {
		queue.SetConflictResolver(o.resolveGitRebaseConflict)
	}
	return o
}

func NewOrchestrator(
	repo domain.StateRepository,
	reg Registry,
	client domain.LLMClient,
	val Validator,
	sched *Scheduler,
	git *GitClient,
	queue *RebaseQueue,
	eval *TestValidator,
	vcsClient domain.VCSClient,
	cfg OrchestratorConfig,
	mailbox *CommandMailbox,
	watchdogRepair RepairHandler,
	promptRenderer PromptRenderer,
	qaDependencies ...QADependencies,
) *Orchestrator {
	var qaCoord *QARuntimeCoordinator
	if len(qaDependencies) > 0 {
		deps := qaDependencies[0]
		qaCoord = NewQARuntimeCoordinator(cfg.QA, client, promptRenderer, deps.WorkspaceFactory,
			deps.ArtifactBuilder, deps.Sandbox, deps.FileSystem, deps.Clock)
	}
	return NewOrchestratorWithRuntime(repo, reg, client, val, sched, git, queue, eval, vcsClient, cfg, OrchestratorRuntimeDependencies{
		Mailbox:        mailbox,
		WatchdogRepair: watchdogRepair,
		PromptRenderer: promptRenderer,
		QA:             qaCoord,
		Observer:       &NoopExecutionReporter{},
	})
}

// Metrics returns the MetricsCollector instance associated with the Orchestrator.
func (o *Orchestrator) Metrics() *MetricsCollector {
	if o == nil {
		return nil
	}
	o.metricsMu.RLock()
	defer o.metricsMu.RUnlock()
	return o.metricsCollector
}

// SetMetricsCollector updates the MetricsCollector instance on the Orchestrator.
func (o *Orchestrator) SetMetricsCollector(mc *MetricsCollector) {
	if o != nil {
		o.metricsMu.Lock()
		o.metricsCollector = mc
		o.metricsMu.Unlock()
	}
}

// metrics returns the current MetricsCollector under the read lock.
func (o *Orchestrator) metrics() *MetricsCollector {
	o.metricsMu.RLock()
	defer o.metricsMu.RUnlock()
	return o.metricsCollector
}

// getStoryStartedAt reads the story wall-clock start time under the mutex.
func (o *Orchestrator) getStoryStartedAt() time.Time {
	o.timesMu.Lock()
	defer o.timesMu.Unlock()
	return o.storyStartedAt
}

// setStoryStartedAt writes the story wall-clock start time under the mutex.
func (o *Orchestrator) setStoryStartedAt(t time.Time) {
	o.timesMu.Lock()
	o.storyStartedAt = t
	o.timesMu.Unlock()
}

// getLastWorkspaceSync reads the last workspace sync time under the mutex.
func (o *Orchestrator) getLastWorkspaceSync() time.Time {
	o.timesMu.Lock()
	defer o.timesMu.Unlock()
	return o.lastWorkspaceSync
}

// setLastWorkspaceSync writes the last workspace sync time under the mutex.
func (o *Orchestrator) setLastWorkspaceSync(t time.Time) {
	o.timesMu.Lock()
	o.lastWorkspaceSync = t
	o.timesMu.Unlock()
}

// SetUnblocker attaches an UnblockerAgent to the Orchestrator. It must be called
// before Start so the goroutine is launched alongside the main polling loop.
func (o *Orchestrator) SetUnblocker(u *UnblockerAgent) {
	if o != nil {
		o.unblocker = u
	}
}

// Start runs the polling loop
func (o *Orchestrator) Start(ctx context.Context) error {
	if o.rebaseQueue != nil {
		go o.rebaseQueue.Start(ctx)
	}
	// Start unblocker goroutine alongside the main polling loop (nil-safe).
	if o.unblocker != nil {
		o.unblocker.Start(ctx)
	}
	for {
		hasWork, err := o.RunOnce(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Orchestrator error: %v\n", err)
		}
		if hasWork && err == nil {
			continue
		}
		var wakeup <-chan struct{}
		if o.mailbox != nil {
			wakeup = o.mailbox.Wakeup()
		}

		combinedWakeup := make(chan struct{}, 1)
		sleepCtx, cancelSleep := context.WithCancel(ctx)
		go func() {
			defer cancelSleep()
			select {
			case <-wakeup:
				select {
				case combinedWakeup <- struct{}{}:
				default:
				}
			case <-o.taskCompletedChan:
				select {
				case combinedWakeup <- struct{}{}:
				default:
				}
			case <-sleepCtx.Done():
			}
		}()

		err = SleepWithInterrupt(sleepCtx, o.cfg.PollInterval, combinedWakeup)
		cancelSleep()
		if err != nil {
			if errors.Is(err, ErrInterrupted) {
				continue
			}
			return err
		}
	}
}

func (o *Orchestrator) updateStateWithRetry(ctx context.Context, updateFn func(state *domain.State) error) error {
	if o.mailbox != nil && o.mailbox.IsRunning() {
		return o.mailbox.SendSync(ctx, &StateMutationCmd{UpdateFn: updateFn})
	}

	maxRetries := o.cfg.OCCMaxRetries
	if maxRetries <= 0 {
		maxRetries = 20
	}
	backoff := o.cfg.OCCBackoffBase
	if backoff <= 0 {
		backoff = 200 * time.Millisecond
	}
	factor := o.cfg.OCCBackoffFactor
	if factor <= 0 {
		factor = 2.0
	}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		state, err := o.repo.Load(ctx)
		if err != nil {
			return err
		}

		if err := updateFn(state); err != nil {
			return err
		}

		err = o.repo.Save(ctx, state)
		if err == nil {
			return nil
		}

		if !errors.Is(err, domain.ErrVersionConflict) {
			return err
		}

		fmt.Printf("⚠️  [OCC Conflict] DB version conflict detected on state update (attempt %d/%d). Retrying...\n", attempt+1, maxRetries)

		if attempt == maxRetries {
			fmt.Printf("❌ [OCC Conflict Exhausted] State update failed after %d retries: %v\n", maxRetries, err)
			return fmt.Errorf("state update failed after %d retries due to OCC conflict: %w", maxRetries, err)
		}

		sleepDur := time.Duration(float64(backoff) * math.Pow(factor, float64(attempt)))
		jitter := 0.5 + rand.Float64()
		sleepDur = time.Duration(float64(sleepDur) * jitter)

		var wakeup <-chan struct{}
		if o.mailbox != nil {
			wakeup = o.mailbox.Wakeup()
		}
		if err := SleepWithInterrupt(ctx, sleepDur, wakeup); err != nil {
			if errors.Is(err, ErrInterrupted) {
				return fmt.Errorf("state update interrupted by incoming command: %w", err)
			}
			return err
		}
	}
	return nil
}

func (o *Orchestrator) markTaskFailed(ctx context.Context, taskID, reason string) error {
	err := o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			if st.Tasks[i].ID == taskID {
				st.Tasks[i].Status = domain.TaskFailed
				st.Tasks[i].UpdatedAt = time.Now()
				break
			}
		}
		domain.AppendAction(st, domain.Action{
			Timestamp: time.Now(),
			Tool:      "execute",
			Success:   false,
			Result:    reason,
		})
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Orchestrator: failed to persist FAILED status for task %s: %v\n", taskID, err)
	}
	return err
}
