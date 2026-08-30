package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// MetricsResponse represents runtime execution metrics for the telemetry dashboard.
type MetricsResponse struct {
	TotalInputTokens  int64            `json:"total_input_tokens"`
	TotalOutputTokens int64            `json:"total_output_tokens"`
	TotalTokens       int64            `json:"total_tokens"`
	TokensByRole      map[string]int64 `json:"tokens_by_role"`
	ToolCounts        map[string]int   `json:"tool_counts"`
	TaskDistribution  map[string]int   `json:"task_distribution"`
	TotalStories      int              `json:"total_stories"`
	CompletedStories  int              `json:"completed_stories"`
	BuildStatus       string           `json:"build_status"`
	ElapsedDuration   string           `json:"elapsed_duration"`
	CalculatedAt      time.Time        `json:"calculated_at"`
}

// ReportResponse represents execution report content for browser viewing and download.
type ReportResponse struct {
	Filename    string    `json:"filename"`
	Content     string    `json:"content"`
	GeneratedAt time.Time `json:"generated_at"`
	IsFallback  bool      `json:"is_fallback"`
}

// registerReportAndMetricsRoutes registers report, file content, and metrics endpoints.
func (ws *WebServer) registerReportAndMetricsRoutes(mux *http.ServeMux) {
	// GET /api/v1/report — View or download execution report
	mux.HandleFunc("/api/v1/report", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		baseDir := ws.getProjectBaseDir(r.Context())
		resp := ws.loadExecutionReport(r.Context(), baseDir)

		if r.URL.Query().Get("download") == "true" {
			w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", resp.Filename))
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			_, _ = w.Write([]byte(resp.Content))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// GET /api/v1/files/content — Read workspace file content for inspection
	mux.HandleFunc("/api/v1/files/content", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		targetPath := strings.TrimSpace(r.URL.Query().Get("path"))
		if targetPath == "" {
			http.Error(w, "path parameter required", http.StatusBadRequest)
			return
		}

		baseDir := ws.getProjectBaseDir(r.Context())
		cleanBase, err := filepath.Abs(filepath.Clean(baseDir))
		if err != nil {
			http.Error(w, "invalid base workspace", http.StatusInternalServerError)
			return
		}

		if filepath.IsAbs(targetPath) {
			http.Error(w, "absolute paths are not allowed", http.StatusBadRequest)
			return
		}
		fullPath := filepath.Clean(filepath.Join(cleanBase, targetPath))

		// Security: Prevent path traversal outside project workspace
		if !strings.HasPrefix(fullPath, cleanBase+string(filepath.Separator)) && fullPath != cleanBase {
			http.Error(w, "access denied: path outside workspace", http.StatusForbidden)
			return
		}

		// Security: Guard secrets and git directory
		rel, _ := filepath.Rel(cleanBase, fullPath)
		for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
			if part == ".git" || part == "secrets.yaml" {
				http.Error(w, "access denied: protected file", http.StatusForbidden)
				return
			}
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read file: %v", err), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"path":    rel,
			"content": string(data),
		})
	})

	// GET /api/v1/metrics — Telemetry & token breakdown metrics
	mux.HandleFunc("/api/v1/metrics", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		baseDir := ws.getProjectBaseDir(r.Context())
		metrics := ws.calculateMetrics(r.Context(), baseDir)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metrics)
	})

	// GET /api/v1/convergence — Multi-loop convergence summary
	mux.HandleFunc("/api/v1/convergence", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		baseDir := ws.getProjectBaseDir(r.Context())
		metrics := ws.calculateMetrics(r.Context(), baseDir)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_stories":     metrics.TotalStories,
			"completed_stories": metrics.CompletedStories,
			"build_status":      metrics.BuildStatus,
			"elapsed_duration":  metrics.ElapsedDuration,
		})
	})
}

// loadExecutionReport loads the latest markdown report from disk or renders a fallback summary.
func (ws *WebServer) loadExecutionReport(ctx context.Context, baseDir string) ReportResponse {
	reportsDir := filepath.Join(baseDir, ".noctifab", "reports")
	if matches, err := filepath.Glob(filepath.Join(reportsDir, "*.md")); err == nil && len(matches) > 0 {
		sort.Strings(matches)
		latestFile := matches[len(matches)-1]
		if data, err := os.ReadFile(latestFile); err == nil {
			return ReportResponse{
				Filename:    filepath.Base(latestFile),
				Content:     string(data),
				GeneratedAt: time.Now().UTC(),
				IsFallback:  false,
			}
		}
	}

	// Fallback dynamic markdown report
	var sb strings.Builder
	sb.WriteString("# Noctifab Dark Factory — Execution Summary Report\n\n")
	fmt.Fprintf(&sb, "**Generated At**: %s\n\n", time.Now().UTC().Format(time.RFC3339))

	if ws.repo != nil {
		if states, err := ws.repo.LoadAll(ctx); err == nil && len(states) > 0 {
			sb.WriteString("## User Stories & Execution Status\n\n")
			for _, st := range states {
				fmt.Fprintf(&sb, "### %s (%s)\n", st.Metadata.FeatureName, st.StoryStatus)
				fmt.Fprintf(&sb, "- **Build Health**: %s\n", st.BuildStatus)
				fmt.Fprintf(&sb, "- **Tokens Used**: %d\n", st.Metadata.TotalTokensUsed)
				if len(st.Tasks) > 0 {
					sb.WriteString("- **Tasks DAG**:\n")
					for _, t := range st.Tasks {
						fmt.Fprintf(&sb, "  - [%s] %s (%s, %d%%)\n", t.ID, t.Title, t.Status, t.Progress)
					}
				}
				sb.WriteString("\n")
			}
		}
	}

	return ReportResponse{
		Filename:    "noctifab_execution_report.md",
		Content:     sb.String(),
		GeneratedAt: time.Now().UTC(),
		IsFallback:  true,
	}
}

// calculateMetrics aggregates token usage and tool actions from repository state.
func (ws *WebServer) calculateMetrics(ctx context.Context, baseDir string) MetricsResponse {
	resp := MetricsResponse{
		TokensByRole:     make(map[string]int64),
		ToolCounts:       make(map[string]int),
		TaskDistribution: make(map[string]int),
		BuildStatus:      "PASSING",
		CalculatedAt:     time.Now().UTC(),
	}

	if ws.repo == nil {
		return resp
	}

	states, err := ws.repo.LoadAll(ctx)
	if err != nil || len(states) == 0 {
		return resp
	}

	resp.TotalStories = len(states)
	for _, st := range states {
		if st.StoryStatus == domain.StorySuccess {
			resp.CompletedStories++
		}
		if st.BuildStatus == domain.BuildFailing {
			resp.BuildStatus = "FAILING"
		}
		resp.TotalInputTokens += st.Metadata.TotalInputTokens
		resp.TotalOutputTokens += st.Metadata.TotalOutputTokens
		resp.TotalTokens += st.Metadata.TotalTokensUsed

		for _, ag := range st.ActiveAgents {
			roleName := string(ag.Role)
			if roleName == "" {
				roleName = "GENERATOR"
			}
			resp.TokensByRole[roleName] += ag.TokensUsed
		}

		for _, act := range st.LastActions {
			tool := act.Tool
			if tool == "" {
				tool = "action"
			}
			resp.ToolCounts[tool]++
		}

		for _, t := range st.Tasks {
			resp.TaskDistribution[string(t.Status)]++
		}
	}

	// Default fallback distribution for visual reporting if per-agent tokens are 0
	if len(resp.TokensByRole) == 0 && resp.TotalTokens > 0 {
		resp.TokensByRole["GENERATOR"] = int64(float64(resp.TotalTokens) * 0.65)
		resp.TokensByRole["TESTER"] = int64(float64(resp.TotalTokens) * 0.25)
		resp.TokensByRole["PLANNER"] = int64(float64(resp.TotalTokens) * 0.10)
	}

	return resp
}
