package usecase_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock LLM client ---

type mockLLMClient struct {
	rawResponse string
	err         error
}

func (m *mockLLMClient) Complete(_ context.Context, _ string) (*domain.LLMResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &domain.LLMResponse{Reasoning: m.rawResponse}, nil
}

// --- fake daemon HTTP server for listener tests ---

type fakeDaemon struct {
	storiesReceived []map[string]any
	status          *domain.State
	clarifications  []usecase.PendingClarification
}

func newFakeDaemon(t *testing.T, fd *fakeDaemon) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	mux.HandleFunc("/statusz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		st := fd.status
		if st == nil {
			st = &domain.State{}
		}
		_ = json.NewEncoder(w).Encode(st)
	})

	mux.HandleFunc("/api/v1/stories", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		fd.storiesReceived = append(fd.storiesReceived, payload)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})

	mux.HandleFunc("/api/v1/clarifications", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fd.clarifications)
	})

	return httptest.NewServer(mux)
}

// --- helper: run listener with fixed stdin, return stdout ---

func runListenerWithDaemon(t *testing.T, llm domain.LLMClient, daemonURL string, stdinLines string) string {
	t.Helper()
	client := usecase.NewDaemonClientWithBase(daemonURL)
	var out bytes.Buffer
	in := strings.NewReader(stdinLines)
	agent := usecase.NewListenerAgent(llm, client, in, &out)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	agent.Start(ctx)
	return out.String()
}

// --- Tests ---

func TestListenerAgent_StartStory_ViaLLM(t *testing.T) {
	t.Run("when the LLM returns START_STORY, it sends the story path to the daemon", func(t *testing.T) {
		fd := &fakeDaemon{}
		srv := newFakeDaemon(t, fd)
		defer srv.Close()

		llm := &mockLLMClient{
			rawResponse: `{"kind":"START_STORY","path":"roadmap/US-0001.md"}`,
		}
		output := runListenerWithDaemon(t, llm, srv.URL, "start roadmap/US-0001.md\n")

		assert.Contains(t, output, "Queuing user story")
		require.Len(t, fd.storiesReceived, 1)
		assert.Equal(t, "roadmap/US-0001.md", fd.storiesReceived[0]["path"])
		assert.Equal(t, false, fd.storiesReceived[0]["directory"])
	})
}

func TestListenerAgent_StartDirectory_ViaLLM(t *testing.T) {
	t.Run("when the LLM returns START_DIRECTORY, it sends the directory path to the daemon", func(t *testing.T) {
		fd := &fakeDaemon{}
		srv := newFakeDaemon(t, fd)
		defer srv.Close()

		llm := &mockLLMClient{
			rawResponse: `{"kind":"START_DIRECTORY","path":"/home/user/repos/project/roadmap"}`,
		}
		output := runListenerWithDaemon(t, llm, srv.URL, "start /home/user/repos/project/roadmap\n")

		assert.Contains(t, output, "Queuing all user stories in directory")
		require.Len(t, fd.storiesReceived, 1)
		assert.Equal(t, true, fd.storiesReceived[0]["directory"])
	})
}

func TestListenerAgent_StartStory_ViaRuleBasedFallback(t *testing.T) {
	t.Run("when LLM fails, the rule-based fallback parses 'start <file.md>' as START_STORY", func(t *testing.T) {
		fd := &fakeDaemon{}
		srv := newFakeDaemon(t, fd)
		defer srv.Close()

		llm := &mockLLMClient{err: assert.AnError}
		output := runListenerWithDaemon(t, llm, srv.URL, "start roadmap/US-0001.md\n")

		assert.Contains(t, output, "Queuing user story")
		assert.Contains(t, output, "roadmap/US-0001.md")
		require.Len(t, fd.storiesReceived, 1)
		assert.Equal(t, false, fd.storiesReceived[0]["directory"])
	})
}

func TestListenerAgent_StartDirectory_ViaRuleBasedFallback(t *testing.T) {
	t.Run("when LLM fails, the rule-based fallback parses 'start <dir>' as START_DIRECTORY", func(t *testing.T) {
		fd := &fakeDaemon{}
		srv := newFakeDaemon(t, fd)
		defer srv.Close()

		llm := &mockLLMClient{err: assert.AnError}
		output := runListenerWithDaemon(t, llm, srv.URL, "start /home/user/repos/project/roadmap\n")

		assert.Contains(t, output, "Queuing all user stories in directory")
		require.Len(t, fd.storiesReceived, 1)
		assert.Equal(t, true, fd.storiesReceived[0]["directory"])
	})
}

func TestListenerAgent_ListStatus_NoTasks(t *testing.T) {
	t.Run("when user types 'status' and daemon has no tasks, it prints the no-tasks message", func(t *testing.T) {
		fd := &fakeDaemon{status: &domain.State{}}
		srv := newFakeDaemon(t, fd)
		defer srv.Close()

		llm := &mockLLMClient{err: assert.AnError}
		output := runListenerWithDaemon(t, llm, srv.URL, "status\n")

		assert.Contains(t, output, "No tasks are currently tracked")
	})
}

func TestListenerAgent_StatusWithTasks(t *testing.T) {
	t.Run("when user types 'status' and tasks exist, it prints the task summary", func(t *testing.T) {
		fd := &fakeDaemon{
			status: &domain.State{
				BuildStatus: domain.BuildPassing,
				Metadata:    domain.StateMetadata{FeatureName: "US-0001.md"},
				Tasks: []domain.Task{
					{ID: "t1", Title: "Setup DB", Status: domain.TaskSuccess},
					{ID: "t2", Title: "Build API", Status: domain.TaskInProgress},
				},
			},
		}
		srv := newFakeDaemon(t, fd)
		defer srv.Close()

		llm := &mockLLMClient{err: assert.AnError}
		output := runListenerWithDaemon(t, llm, srv.URL, "status\n")

		assert.Contains(t, output, "US-0001.md")
		assert.Contains(t, output, "Setup DB")
		assert.Contains(t, output, "Build API")
	})
}

func TestListenerAgent_UnknownCommand(t *testing.T) {
	t.Run("when user types garbage, it prints an unknown command hint", func(t *testing.T) {
		fd := &fakeDaemon{}
		srv := newFakeDaemon(t, fd)
		defer srv.Close()

		llm := &mockLLMClient{err: assert.AnError}
		output := runListenerWithDaemon(t, llm, srv.URL, "blorp blorp\n")

		assert.Contains(t, output, "did not understand")
	})
}

func TestListenerAgent_EmptyLine_ShowsPromptAgain(t *testing.T) {
	t.Run("when user submits empty lines, the listener skips them and shows the prompt again", func(t *testing.T) {
		fd := &fakeDaemon{}
		srv := newFakeDaemon(t, fd)
		defer srv.Close()

		llm := &mockLLMClient{err: assert.AnError}
		output := runListenerWithDaemon(t, llm, srv.URL, "\n\n\n")

		require.NotEmpty(t, output, "output should contain at least the initial prompt")
		assert.Contains(t, output, ">")
	})
}
