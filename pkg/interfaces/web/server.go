package web

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

//go:embed static/*
var StaticFS embed.FS

// WebServerConfig holds runtime settings for the embedded web dashboard.
type WebServerConfig struct {
	Host     string
	Port     int
	ReadOnly bool
}

// WebServer coordinates HTTP dashboard endpoints and live SSE streaming.
type WebServer struct {
	config      WebServerConfig
	repo        domain.StateRepository
	mailbox     *services.CommandMailbox
	broadcaster *SSEBroadcaster
	storyCh     chan<- services.StoryWorkItem
	httpServer  *http.Server
}

// NewWebServer initializes a new WebServer instance.
func NewWebServer(cfg WebServerConfig, repo domain.StateRepository, mailbox *services.CommandMailbox, broadcaster *SSEBroadcaster) *WebServer {
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == 0 {
		cfg.Port = 8080
	}
	if broadcaster == nil {
		broadcaster = NewSSEBroadcaster(100)
	}

	ws := &WebServer{
		config:      cfg,
		repo:        repo,
		mailbox:     mailbox,
		broadcaster: broadcaster,
	}

	mux := ws.buildMux()
	ws.httpServer = &http.Server{
		Addr:              fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // Disabled for long-lived SSE streaming connections
		IdleTimeout:       120 * time.Second,
	}

	return ws
}

// Start launches the web server in a background goroutine.
func (ws *WebServer) Start() error {
	go func() {
		_ = ws.httpServer.ListenAndServe()
	}()
	return nil
}

// Shutdown gracefully stops the web server.
func (ws *WebServer) Shutdown(ctx context.Context) error {
	if ws.httpServer != nil {
		return ws.httpServer.Shutdown(ctx)
	}
	return nil
}

// Addr returns the configured listening address.
func (ws *WebServer) Addr() string {
	return ws.httpServer.Addr
}

// Broadcaster returns the associated SSE broadcaster for event publishing.
func (ws *WebServer) Broadcaster() *SSEBroadcaster {
	return ws.broadcaster
}

// SetStoryChannel connects a background work item queue to the web server.
func (ws *WebServer) SetStoryChannel(ch chan<- services.StoryWorkItem) {
	ws.storyCh = ch
}

func (ws *WebServer) buildMux() *http.ServeMux {
	mux := http.NewServeMux()

	// 1. Static asset routes
	staticContent, err := fs.Sub(StaticFS, "static")
	if err == nil {
		fileServer := http.FileServer(http.FS(staticContent))
		mux.Handle("/", fileServer)
	}

	// 2. GET /api/v1/state — JSON snapshot of current system state
	mux.HandleFunc("/api/v1/state", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ws.repo == nil {
			http.Error(w, "state repository unavailable", http.StatusServiceUnavailable)
			return
		}
		state, err := ws.repo.Load(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state)
	})

	// 3. GET /api/v1/events — Server-Sent Events (SSE) stream
	mux.HandleFunc("/api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		ch := ws.broadcaster.Subscribe(0)
		defer ws.broadcaster.Unsubscribe(ch)

		// Send initial state snapshot if repository exists
		if ws.repo != nil {
			if st, err := ws.repo.Load(r.Context()); err == nil {
				if b, err := json.Marshal(st); err == nil {
					_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
					flusher.Flush()
				}
			}
		}

		// Keepalive ticker to prevent reverse-proxy timeouts
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
				_, _ = fmt.Fprint(w, ":keepalive\n\n")
				flusher.Flush()
			case ev, ok := <-ch:
				if !ok {
					return
				}
				_, _ = fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, string(ev.Payload))
				flusher.Flush()
			}
		}
	})

	// 4. POST /api/v1/steer — Submit steering directive
	mux.HandleFunc("/api/v1/steer", func(w http.ResponseWriter, r *http.Request) {
		if ws.config.ReadOnly {
			http.Error(w, "server is in read-only mode", http.StatusForbidden)
			return
		}
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

		if ws.mailbox != nil {
			ws.mailbox.Send(&services.SteerCmd{
				TaskID:    payload.TaskID,
				Directive: payload.Directive,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})

	// 5. POST /api/v1/orders — Submit new feature order / prompt
	mux.HandleFunc("/api/v1/orders", func(w http.ResponseWriter, r *http.Request) {
		if ws.config.ReadOnly {
			http.Error(w, "server is in read-only mode", http.StatusForbidden)
			return
		}
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

		if ws.mailbox != nil {
			ws.mailbox.Send(&services.OrderCmd{
				Prompt:  payload.Prompt,
				StoryCh: ws.storyCh,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})

	// 6. POST /api/v1/pause — Pause execution
	mux.HandleFunc("/api/v1/pause", func(w http.ResponseWriter, r *http.Request) {
		if ws.config.ReadOnly {
			http.Error(w, "server is in read-only mode", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ws.repo != nil {
			err := services.SaveStateWithBackoff(r.Context(), ws.repo, func(st *domain.State) {
				st.StoryStatus = domain.StoryPaused
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"paused"}`))
	})

	// 7. POST /api/v1/resume — Resume execution
	mux.HandleFunc("/api/v1/resume", func(w http.ResponseWriter, r *http.Request) {
		if ws.config.ReadOnly {
			http.Error(w, "server is in read-only mode", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ws.repo != nil {
			err := services.SaveStateWithBackoff(r.Context(), ws.repo, func(st *domain.State) {
				st.StoryStatus = domain.StoryRunning
			})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"resumed"}`))
	})

	// 8. GET /api/v1/clarifications — List clarifications
	mux.HandleFunc("/api/v1/clarifications", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ws.repo == nil {
			http.Error(w, "state repository unavailable", http.StatusServiceUnavailable)
			return
		}
		state, err := ws.repo.Load(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		clarifications := state.Clarifications
		if r.URL.Query().Get("pending") == "true" {
			pending := make([]domain.Clarification, 0)
			for _, c := range clarifications {
				if !c.Resolved {
					pending = append(pending, c)
				}
			}
			clarifications = pending
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(clarifications)
	})

	// 9. POST /api/v1/clarifications/ — Resolve clarification by ID
	mux.HandleFunc("/api/v1/clarifications/", func(w http.ResponseWriter, r *http.Request) {
		if ws.config.ReadOnly {
			http.Error(w, "server is in read-only mode", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/clarifications/")
		parts := strings.Split(path, "/")
		if len(parts) == 0 || parts[0] == "" {
			http.Error(w, "clarification id required", http.StatusBadRequest)
			return
		}
		id := parts[0]

		var body struct {
			Answer string `json:"answer"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(body.Answer) == "" {
			http.Error(w, "answer cannot be empty", http.StatusBadRequest)
			return
		}

		if ws.mailbox != nil {
			ws.mailbox.Send(&services.ResolveClarificationCmd{
				ID:     id,
				Answer: body.Answer,
			})
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"accepted"}`))
	})

	// 10. GET /api/v1/orders — List persisted prompt orders
	mux.HandleFunc("/api/v1/orders/list", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if ws.repo == nil {
			http.Error(w, "state repository unavailable", http.StatusServiceUnavailable)
			return
		}
		state, err := ws.repo.Load(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(state.Orders)
	})

	// 11. /api/v1/spec routes — Visual Web Spec Editor
	ws.registerSpecRoutes(mux)

	// 12. /api/v1/roadmap routes — User Stories & DoD metadata
	ws.registerRoadmapRoutes(mux)

	// 13. /api/v1/report and /api/v1/metrics routes
	ws.registerReportAndMetricsRoutes(mux)

	return mux
}
