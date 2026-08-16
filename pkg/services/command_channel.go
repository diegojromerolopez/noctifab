package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ErrInterrupted is returned by SleepWithInterrupt when the wakeup channel fires.
var ErrInterrupted = errors.New("operation interrupted by incoming command")

// Command represents a state mutation operation routed through the CommandMailbox.
type Command interface {
	Execute(ctx context.Context, repo domain.StateRepository) error
}

// CommandMailbox serializes state changes to prevent database write conflicts.
type CommandMailbox struct {
	repo   domain.StateRepository
	cmds   chan Command
	wakeup chan struct{}
}

func NewCommandMailbox(repo domain.StateRepository) *CommandMailbox {
	return &CommandMailbox{
		repo:   repo,
		cmds:   make(chan Command, 100),
		wakeup: make(chan struct{}, 1),
	}
}

// Start processes mailbox commands sequentially in a single loop
func (m *CommandMailbox) Start(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case cmd := <-m.cmds:
			if err := cmd.Execute(ctx, m.repo); err != nil {
				// Log command execution failure
				fmt.Printf("Command execution error: %v\n", err)
			}
		}
	}
}

func (m *CommandMailbox) Send(cmd Command) {
	m.cmds <- cmd
	select {
	case m.wakeup <- struct{}{}:
	default:
	}
}

// Wakeup returns a channel that fires when a command is sent to the mailbox.
func (m *CommandMailbox) Wakeup() <-chan struct{} {
	return m.wakeup
}

// PopAll drains and returns all currently buffered commands in the mailbox.
func (m *CommandMailbox) PopAll() []Command {
	var list []Command
	for {
		select {
		case cmd := <-m.cmds:
			list = append(list, cmd)
		default:
			return list
		}
	}
}

// SleepWithInterrupt sleeps for the given duration or until the context is cancelled
// or a command notification arrives on the wakeup channel.
func SleepWithInterrupt(ctx context.Context, duration time.Duration, wakeup <-chan struct{}) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	case <-wakeup:
		return ErrInterrupted
	}
}

// ResolveClarificationCmd is sent to resolve a clarification question.
type ResolveClarificationCmd struct {
	ID     string
	Answer string
}

func (c *ResolveClarificationCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return err
	}

	for i := range state.Clarifications {
		// Matching on either custom ID or resolving matching questions
		if state.Clarifications[i].Question == c.ID || fmt.Sprintf("%d", i) == c.ID {
			state.Clarifications[i].Answer = c.Answer
			state.Clarifications[i].Resolved = true
			return repo.Save(ctx, state)
		}
	}
	return errors.New("clarification not found")
}

// AddTaskCmd is sent to dynamically add a task.
type AddTaskCmd struct {
	Task domain.Task
}

func (c *AddTaskCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return err
	}
	state.Tasks = append(state.Tasks, c.Task)
	_ = WriteTaskMarkdown(state.ProjectPath, state.Metadata.InputPath, c.Task)
	return repo.Save(ctx, state)
}

// OverrideMergeCmd manually forces success status on a blocked task.
type OverrideMergeCmd struct {
	TaskID string
}

func (c *OverrideMergeCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	state, err := repo.Load(ctx)
	if err != nil {
		return err
	}
	for i := range state.Tasks {
		if state.Tasks[i].ID == c.TaskID {
			state.Tasks[i].Status = domain.TaskSuccess
			state.Tasks[i].UpdatedAt = time.Now()
			return repo.Save(ctx, state)
		}
	}
	return domain.ErrTaskNotFound
}

// StartDaemonServer sets up local loopback REST API bindings.
// storyCh is the channel into which story work items are forwarded when the daemon
// operates in server mode; pass nil when running in single-story mode.
func StartDaemonServer(repo domain.StateRepository, mailbox *CommandMailbox, storyCh chan<- StoryWorkItem, llmClient domain.LLMClient, renderer PromptRenderer) *http.Server {
	mux := newDaemonMux(repo, mailbox, storyCh, llmClient, renderer)

	// Enforce loopback binding (127.0.0.1)
	server := &http.Server{
		Addr:              "127.0.0.1:18080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		_ = server.ListenAndServe()
	}()

	return server
}

// newDaemonMux builds the REST API routes for the daemon HTTP server.
func newDaemonMux(repo domain.StateRepository, mailbox *CommandMailbox, storyCh chan<- StoryWorkItem, llmClient domain.LLMClient, renderer PromptRenderer) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})

	mux.HandleFunc("/statusz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		state, err := repo.Load(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Strip the workspace file index and cap the action log: both can be
		// large and are not needed for daemon status inspection.
		state.Files = nil
		if len(state.LastActions) > 20 {
			state.LastActions = state.LastActions[len(state.LastActions)-20:]
		}
		_ = json.NewEncoder(w).Encode(state)
	})

	// GET /api/v1/clarifications?pending=true  — returns unresolved clarifications
	// POST /api/v1/clarifications/{id}/resolve  — resolves a clarification by ID
	mux.HandleFunc("/api/v1/clarifications", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		state, err := repo.Load(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		pendingOnly := r.URL.Query().Get("pending") == "true"
		result := make([]map[string]any, 0)
		for i, c := range state.Clarifications {
			if pendingOnly && c.Resolved {
				continue
			}
			result = append(result, map[string]any{
				"id":       fmt.Sprintf("%d", i),
				"question": c.Question,
				"resolved": c.Resolved,
				"answer":   c.Answer,
				"asked_at": c.AskedAt,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})

	mux.HandleFunc("/api/v1/clarifications/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := r.URL.Path
		parts := strings.Split(path, "/")
		if len(parts) < 6 || parts[5] != "resolve" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		id := parts[4]

		var body struct {
			Answer string `json:"answer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		mailbox.Send(&ResolveClarificationCmd{ID: id, Answer: body.Answer})
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})

	// POST /api/v1/stories — enqueue a user story or directory in server mode
	mux.HandleFunc("/api/v1/stories", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if storyCh == nil {
			http.Error(w, "server not in story-queue mode", http.StatusServiceUnavailable)
			return
		}
		var payload struct {
			Path      string `json:"path"`
			Directory bool   `json:"directory"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		isDir := payload.Directory
		if !isDir && payload.Path != "" {
			if fi, err := os.Stat(payload.Path); err == nil && fi.IsDir() {
				isDir = true
			}
		}
		if isDir {
			mailbox.Send(&StartDirectoryCmd{DirPath: payload.Path, StoryCh: storyCh, LLMClient: llmClient, Renderer: renderer})
		} else {
			mailbox.Send(&StartUserStoryCmd{Path: payload.Path, StoryCh: storyCh, LLMClient: llmClient, Renderer: renderer})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})

	// GET /api/v1/status — list lightweight status summaries of all user
	// stories (no Files, no LastActions, no full task bodies).
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		summaries, err := repo.LoadAllSummaries(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if summaries == nil {
			summaries = []domain.StateSummary{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(summaries)
	})

	// POST /api/v1/pause — pause the active user story
	mux.HandleFunc("/api/v1/pause", storyStatusHandler(repo, domain.StoryPaused, `{"status":"paused"}`))

	// POST /api/v1/resume — resume the paused user story
	mux.HandleFunc("/api/v1/resume", storyStatusHandler(repo, domain.StoryRunning, `{"status":"resumed"}`))

	// POST /api/v1/cancel — cancel the active user story
	mux.HandleFunc("/api/v1/cancel", storyStatusHandler(repo, domain.StoryCancelled, `{"status":"cancelled"}`))

	// POST /api/v1/tasks — add a manual task
	mux.HandleFunc("/api/v1/tasks", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			DependsOn   []string `json:"depends_on"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		task := domain.Task{
			ID:          fmt.Sprintf("task-%d", time.Now().UnixNano()),
			Title:       payload.Title,
			Description: payload.Description,
			DependsOn:   payload.DependsOn,
			Status:      domain.TaskPending,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		mailbox.Send(&AddTaskCmd{Task: task})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(task)
	})

	mux.HandleFunc("/api/v1/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := r.URL.Path
		parts := strings.Split(path, "/")
		if len(parts) < 6 || parts[5] != "override-merge" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		id := parts[4]

		mailbox.Send(&OverrideMergeCmd{TaskID: id})
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})

	registerSteerAndOrderRoutes(mux, mailbox, storyCh, llmClient, renderer)

	return mux
}
