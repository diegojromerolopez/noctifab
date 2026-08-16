package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// SteerCmd injects a developer steering directive into the target task or active running tasks.
type SteerCmd struct {
	TaskID    string `json:"task_id,omitempty"`
	Directive string `json:"directive"`
}

func (c *SteerCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	if strings.TrimSpace(c.Directive) == "" {
		return errors.New("directive cannot be empty")
	}

	state, err := repo.Load(ctx)
	if err != nil {
		return err
	}

	matched := false
	if c.TaskID != "" {
		for i := range state.Tasks {
			if state.Tasks[i].ID == c.TaskID {
				state.Tasks[i].UserDirectives = append(state.Tasks[i].UserDirectives, c.Directive)
				state.Tasks[i].UpdatedAt = time.Now()
				matched = true
				break
			}
		}
	} else {
		// If no specific TaskID, attach to any running tasks or the latest pending task
		for i := range state.Tasks {
			if state.Tasks[i].Status == domain.TaskInProgress {
				state.Tasks[i].UserDirectives = append(state.Tasks[i].UserDirectives, c.Directive)
				state.Tasks[i].UpdatedAt = time.Now()
				matched = true
			}
		}
		if !matched && len(state.Tasks) > 0 {
			// Attach to the last task
			idx := len(state.Tasks) - 1
			state.Tasks[idx].UserDirectives = append(state.Tasks[idx].UserDirectives, c.Directive)
			state.Tasks[idx].UpdatedAt = time.Now()
			matched = true
		}
	}

	if !matched && len(state.Tasks) == 0 {
		// Record in last actions as a directive
		state.LastActions = append(state.LastActions, domain.Action{
			Tool:      "STEER",
			Reasoning: c.Directive,
			Result:    "User steering directive queued",
			Success:   true,
			Timestamp: time.Now(),
		})
	}

	return repo.Save(ctx, state)
}

// OrderCmd creates and enqueues a new story / feature prompt order from user input.
type OrderCmd struct {
	Prompt    string
	StoryCh   chan<- StoryWorkItem
	LLMClient domain.LLMClient
	Renderer  PromptRenderer
}

func (c *OrderCmd) Execute(ctx context.Context, repo domain.StateRepository) error {
	trimmed := strings.TrimSpace(c.Prompt)
	if trimmed == "" {
		return errors.New("order prompt cannot be empty")
	}

	state, err := repo.Load(ctx)
	if err != nil {
		return err
	}

	baseDir := state.ProjectPath
	if baseDir == "" {
		baseDir = "."
	}

	// Create story file in .noctifab/stories/
	storiesDir := filepath.Join(baseDir, ".noctifab", "stories")
	if err := os.MkdirAll(storiesDir, 0755); err != nil {
		return fmt.Errorf("failed to create stories directory: %w", err)
	}

	timestamp := time.Now().Format("20060102_150405")
	storyPath := filepath.Join(storiesDir, fmt.Sprintf("story_%s.md", timestamp))
	content := fmt.Sprintf("# User Order: %s\n\n## Description\n%s\n", trimmed, trimmed)
	if err := os.WriteFile(storyPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write story order file: %w", err)
	}

	if c.StoryCh != nil {
		c.StoryCh <- StoryWorkItem{
			Path: storyPath,
		}
	}

	return nil
}

// registerSteerAndOrderRoutes adds /api/v1/steer and /api/v1/orders routes to daemon mux.
func registerSteerAndOrderRoutes(mux *http.ServeMux, mailbox *CommandMailbox, storyCh chan<- StoryWorkItem, llmClient domain.LLMClient, renderer PromptRenderer) {
	// POST /api/v1/steer — inject mid-flight steering directive
	mux.HandleFunc("/api/v1/steer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			TaskID    string `json:"task_id,omitempty"`
			Directive string `json:"directive"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(payload.Directive) == "" {
			http.Error(w, "directive cannot be empty", http.StatusBadRequest)
			return
		}

		mailbox.Send(&SteerCmd{
			TaskID:    payload.TaskID,
			Directive: payload.Directive,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})

	// POST /api/v1/orders — enqueue a prompt order directly
	mux.HandleFunc("/api/v1/orders", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var payload struct {
			Prompt string `json:"prompt"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(payload.Prompt) == "" {
			http.Error(w, "prompt cannot be empty", http.StatusBadRequest)
			return
		}

		mailbox.Send(&OrderCmd{
			Prompt:    payload.Prompt,
			StoryCh:   storyCh,
			LLMClient: llmClient,
			Renderer:  renderer,
		})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})
}
