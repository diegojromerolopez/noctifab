package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// RepairHandler defines the contract for automatic repair of hung/failed test suites.
type RepairHandler interface {
	AttemptRepair(ctx context.Context, state *domain.State, task domain.Task, watchdogOutput string, watchdogErr error) (*RepairResult, error)
}

type OrchestratorConfig struct {
	Architecture          string
	ArchitectNumber       int
	ArchitectIterations   int
	GeneratorsNumber      int
	GeneratorsIterations  int
	TestersNumber         int
	TestersIterations     int
	QAAgentsNumber        int
	QAAgentsIterations    int
	SecurityNumber        int
	SecurityIterations    int
	PerformanceNumber     int
	PerformanceIterations int
	DocsNumber            int
	DocsIterations        int
	DevOpsNumber          int
	DevOpsIterations      int
	PollInterval          time.Duration
	MaxRetries            int
	Concurrency           int
	UseWorktrees          bool
	OCCMaxRetries         int
	OCCBackoffBase        time.Duration
	OCCBackoffFactor      float64
	MaxDuration           time.Duration
	AutoCreatePR          bool
	MaxActions            int
	ExcludePaths          []string
	MetricsEnabled        bool
	MetricsOutputPath     string
	Context               config.ContextConfig
	WorkspaceCache        config.WorkspaceCacheConfig
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
	metricsCollector  *MetricsCollector
	unblocker         *UnblockerAgent
	storyStartedAt    time.Time
	totalActions      int64
	taskCompletedChan chan struct{}
	lastWorkspaceSync time.Time
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
) *Orchestrator {
	return &Orchestrator{
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
		mailbox:           mailbox,
		watchdogRepair:    watchdogRepair,
		metricsCollector:  NewMetricsCollector(cfg.MetricsEnabled),
		taskCompletedChan: make(chan struct{}, 100),
	}
}

// Metrics returns the MetricsCollector instance associated with the Orchestrator.
func (o *Orchestrator) Metrics() *MetricsCollector {
	if o == nil {
		return nil
	}
	return o.metricsCollector
}

// SetMetricsCollector updates the MetricsCollector instance on the Orchestrator.
func (o *Orchestrator) SetMetricsCollector(mc *MetricsCollector) {
	if o != nil {
		o.metricsCollector = mc
	}
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

// RunOnce runs a single cycle of the event loop.
// Returns a boolean indicating if any ready tasks were executed in this cycle, and any error.
func (o *Orchestrator) RunOnce(ctx context.Context) (bool, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "RunOnce",
		trace.WithAttributes(
			attribute.Int("concurrency", o.cfg.Concurrency),
			attribute.Int("occ_max_retries", o.cfg.OCCMaxRetries),
			attribute.String("poll_interval", o.cfg.PollInterval.String()),
		))
	defer span.End()

	state, err := o.repo.Load(ctx)
	if err != nil {
		return false, err
	}

	// 1. Observe Phase: File indexing and sync
	if err := o.syncWorkspaceFiles(ctx, state); err != nil {
		return false, err
	}

	// 2. Scheduler check: find ready tasks
	ready := o.scheduler.GetReadyTasks(state, o.cfg.Concurrency)

	// 2a. Global max_actions enforcement.
	currentActions := atomic.LoadInt64(&o.totalActions)
	if o.cfg.MaxActions > 0 && int(currentActions) >= o.cfg.MaxActions && state.StoryStatus == domain.StoryIdle {
		fmt.Printf("Orchestrator: story exceeded max_actions ceiling %d (executed %d); failing remaining tasks and aborting story.\n", o.cfg.MaxActions, currentActions)
		_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
			for i := range st.Tasks {
				if st.Tasks[i].Status != domain.TaskSuccess && st.Tasks[i].Status != domain.TaskFailed {
					st.Tasks[i].Status = domain.TaskFailed
					st.Tasks[i].FailureLog = fmt.Sprintf("story exceeded max_actions ceiling %d (executed %d)", st.Tasks[i].MaxRetries, currentActions)
					st.Tasks[i].UpdatedAt = time.Now()
				}
			}
			st.BuildStatus = domain.BuildFailing
			st.StoryStatus = domain.StoryFailed
			return nil
		})
		return false, nil
	}

	// 2b. Story-level wall clock enforcement. When max_duration is configured
	// (> 0) and the story has been running longer than the limit, fail every
	// non-finished task and mark the story as FAILED so the daemon stops
	// spending LLM budget on a stuck story. The start time is the first cycle
	// in which any task became ready.
	if o.cfg.MaxDuration > 0 && len(state.Tasks) > 0 {
		if o.storyStartedAt.IsZero() && len(ready) > 0 {
			o.storyStartedAt = time.Now()
		}
		// Enforce the wall-clock cap regardless of the transient StoryStatus:
		// previously this only fired while StoryIdle, so a story that was stuck
		// mid-execution (StoryRunning) with a hung LLM/sandbox call never hit
		// the deadline and burned the remaining budget indefinitely. Now we
		// abort as soon as the deadline elapses and any task is still pending.
		if !o.storyStartedAt.IsZero() && time.Since(o.storyStartedAt) > o.cfg.MaxDuration && !o.allTasksFinished(state) {
			elapsed := time.Since(o.storyStartedAt)
			fmt.Printf("Orchestrator: story exceeded max_duration %s (elapsed %s); failing remaining tasks and aborting story.\n", o.cfg.MaxDuration, elapsed.Truncate(time.Second))
			_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
				for i := range st.Tasks {
					if st.Tasks[i].Status != domain.TaskSuccess && st.Tasks[i].Status != domain.TaskFailed {
						st.Tasks[i].Status = domain.TaskFailed
						st.Tasks[i].FailureLog = fmt.Sprintf("story exceeded max_duration %s (elapsed %s)", o.cfg.MaxDuration, elapsed.Truncate(time.Second))
						st.Tasks[i].UpdatedAt = time.Now()
					}
				}
				st.BuildStatus = domain.BuildFailing
				st.StoryStatus = domain.StoryFailed
				return nil
			})
			return false, nil
		}
	}

	if len(ready) == 0 {
		// Finalize exactly once: guarded by StoryStatus (not BuildStatus, which
		// may have been set to FAILING mid-run by a failing task and could
		// recover on retry). StoryStatus transitions Idle -> Success/Failed
		// exactly once when all tasks are finished.
		if o.allTasksFinished(state) && state.StoryStatus == domain.StoryIdle {
			buildOK := o.allTasksSucceeded(state)
			if buildOK {
				if finalErr := o.FinalizeUserStory(ctx, state); finalErr != nil {
					fmt.Fprintf(os.Stderr, "Orchestrator: finalization failed: %v\n", finalErr)
				}
			} else {
				fmt.Printf("Orchestrator: one or more tasks failed test validation; marking build as FAILING and skipping release finalization.\n")
			}
			_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
				if buildOK {
					st.BuildStatus = domain.BuildPassing
					st.StoryStatus = domain.StorySuccess
				} else {
					st.BuildStatus = domain.BuildFailing
					st.StoryStatus = domain.StoryFailed
				}
				return nil
			})
		}
		return false, nil
	}

	fmt.Printf("Orchestrator: Found %d ready task(s) to execute in this cycle\n", len(ready))

	var wg sync.WaitGroup
	// 3. Dispatch ready tasks
	for _, task := range ready {
		wg.Add(1)
		go func(t domain.Task) {
			defer wg.Done()
			o.executeTask(ctx, state.ID, t.ID)
		}(task)
	}

	wg.Wait()
	return true, nil
}

func (o *Orchestrator) updateStateWithRetry(ctx context.Context, updateFn func(state *domain.State) error) error {
	maxRetries := o.cfg.OCCMaxRetries
	if maxRetries <= 0 {
		maxRetries = 5
	}
	backoff := o.cfg.OCCBackoffBase
	if backoff <= 0 {
		backoff = 50 * time.Millisecond
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

func (o *Orchestrator) markTaskFailed(ctx context.Context, taskID, reason string) {
	_ = o.updateStateWithRetry(ctx, func(st *domain.State) error {
		for i := range st.Tasks {
			if st.Tasks[i].ID == taskID {
				st.Tasks[i].Status = domain.TaskFailed
				st.Tasks[i].UpdatedAt = time.Now()
				break
			}
		}
		st.LastActions = append(st.LastActions, domain.Action{
			Timestamp: time.Now(),
			Tool:      "execute",
			Success:   false,
			Result:    reason,
		})
		return nil
	})
}
