package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/storage"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"gopkg.in/yaml.v3"
)

// SpecResponse represents the JSON response structure for /api/v1/spec endpoints.
type SpecResponse struct {
	Content           string            `json:"content"`
	ActiveVersion     int               `json:"active_version"`
	AvailableVersions []int             `json:"available_versions"`
	LatestDiff        string            `json:"latest_diff,omitempty"`
	IsApproved        bool              `json:"is_approved"`
	TargetFile        string            `json:"target_file"`
	UpdatedAt         time.Time         `json:"updated_at"`
	ModelRoles        map[string]string `json:"model_roles"`
}

// registerSpecRoutes registers the HTTP handlers for the Visual Spec Editor.
func (ws *WebServer) registerSpecRoutes(mux *http.ServeMux) {
	// GET /api/v1/spec — Get current SPEC.md, version list, and metadata
	mux.HandleFunc("/api/v1/spec", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		baseDir := ws.getProjectBaseDir(r.Context())
		specPath := filepath.Join(baseDir, "SPEC.md")

		data, err := os.ReadFile(specPath)
		content := ""
		if err == nil {
			content = string(data)
		}

		snapshots := storage.NewSpecSnapshotManager(baseDir)
		versions, _ := snapshots.ListSnapshots()
		activeVer := len(versions)
		if activeVer == 0 && content != "" {
			activeVer = 1
			versions = []int{1}
		}

		cfg, _ := ws.loadConfig(baseDir)
		roles := ws.getRoleAttributions(cfg)

		resp := SpecResponse{
			Content:           content,
			ActiveVersion:     activeVer,
			AvailableVersions: versions,
			TargetFile:        specPath,
			UpdatedAt:         time.Now().UTC(),
			ModelRoles:        roles,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// POST /api/v1/spec/refine — Refine SPEC.md with human feedback
	mux.HandleFunc("/api/v1/spec/refine", func(w http.ResponseWriter, r *http.Request) {
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

		baseDir := ws.getProjectBaseDir(r.Context())
		specPath := filepath.Join(baseDir, "SPEC.md")

		existingData, _ := os.ReadFile(specPath)
		currentSpec := string(existingData)

		cfg, _ := ws.loadConfig(baseDir)
		router := llm.NewResilientLLMRouter(cfg, nil)
		pipeline := services.NewSpecMultiAgentPipeline(cfg, router, nil)
		auditor := services.NewSpecConsensusAuditor(cfg, router, nil)

		var updatedSpec string
		var err error
		if currentSpec == "" {
			updatedSpec, err = pipeline.ExecutePass(r.Context(), payload.Prompt, "")
		} else {
			updatedSpec, err = pipeline.ExecuteRefinePass(r.Context(), currentSpec, payload.Prompt, nil)
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("spec refinement failed: %v", err), http.StatusInternalServerError)
			return
		}

		// Run consensus audit pass
		if cfg == nil || cfg.Spec.IsConsensusEnabled() {
			if reconciled, aErr := auditor.AuditAndReconcile(r.Context(), updatedSpec, payload.Prompt); aErr == nil {
				updatedSpec = reconciled
			}
		}

		_ = os.WriteFile(specPath, []byte(updatedSpec), 0644)

		renderer := services.NewSpecRenderer()
		diff := renderer.CalculateDiff(currentSpec, updatedSpec)

		snapshots := storage.NewSpecSnapshotManager(baseDir)
		versions, _ := snapshots.ListSnapshots()
		nextVersion := len(versions) + 1
		_, _, _ = snapshots.SaveSnapshot(nextVersion, updatedSpec, diff)
		versions = append(versions, nextVersion)

		resp := SpecResponse{
			Content:           updatedSpec,
			ActiveVersion:     nextVersion,
			AvailableVersions: versions,
			LatestDiff:        diff,
			TargetFile:        specPath,
			UpdatedAt:         time.Now().UTC(),
			ModelRoles:        ws.getRoleAttributions(cfg),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// POST /api/v1/spec/checkout — Time-travel checkout to a specific version
	mux.HandleFunc("/api/v1/spec/checkout", func(w http.ResponseWriter, r *http.Request) {
		if ws.config.ReadOnly {
			http.Error(w, "server is in read-only mode", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload struct {
			Version int `json:"version"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		baseDir := ws.getProjectBaseDir(r.Context())
		snapshots := storage.NewSpecSnapshotManager(baseDir)

		specContent, err := snapshots.LoadSnapshot(payload.Version)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to load version %d: %v", payload.Version, err), http.StatusNotFound)
			return
		}

		specPath := filepath.Join(baseDir, "SPEC.md")
		_ = os.WriteFile(specPath, []byte(specContent), 0644)

		versions, _ := snapshots.ListSnapshots()
		cfg, _ := ws.loadConfig(baseDir)

		resp := SpecResponse{
			Content:           specContent,
			ActiveVersion:     payload.Version,
			AvailableVersions: versions,
			TargetFile:        specPath,
			UpdatedAt:         time.Now().UTC(),
			ModelRoles:        ws.getRoleAttributions(cfg),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	// POST /api/v1/spec/approve — Finalize spec & trigger roadmap generation
	mux.HandleFunc("/api/v1/spec/approve", func(w http.ResponseWriter, r *http.Request) {
		if ws.config.ReadOnly {
			http.Error(w, "server is in read-only mode", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		baseDir := ws.getProjectBaseDir(r.Context())
		specPath := filepath.Join(baseDir, "SPEC.md")

		if _, err := os.Stat(specPath); os.IsNotExist(err) {
			http.Error(w, "SPEC.md does not exist yet", http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"approved","message":"SPEC.md approved and ready for Dark Factory execution"}`))
	})
}

func (ws *WebServer) getProjectBaseDir(ctx context.Context) string {
	if ws.repo != nil {
		if st, err := ws.repo.Load(ctx); err == nil && st != nil && st.ProjectPath != "" {
			return st.ProjectPath
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func (ws *WebServer) loadConfig(baseDir string) (*config.Config, error) {
	cfgPath := filepath.Join(baseDir, ".noctifab", "config.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil && len(data) > 0 {
		cfg := config.DefaultConfig()
		_ = yaml.Unmarshal(data, cfg)
		return cfg, nil
	}
	return config.DefaultConfig(), nil
}

func (ws *WebServer) getRoleAttributions(cfg *config.Config) map[string]string {
	roles := map[string]string{
		"Product Manager":   "Default Provider",
		"Systems Architect": "Default Provider",
		"Test Architect":    "Default Provider",
		"QA Specialist":     "Default Provider",
		"Consensus Auditor": "Multi-Model Cross Audit",
	}
	if cfg == nil {
		return roles
	}
	if len(cfg.Agents.ProductManager.Providers) > 0 {
		roles["Product Manager"] = cfg.Agents.ProductManager.Providers[0].Name
	}
	if len(cfg.Agents.Generators.Providers) > 0 {
		roles["Systems Architect"] = cfg.Agents.Generators.Providers[0].Name
	}
	if len(cfg.Agents.Testers.Providers) > 0 {
		roles["Test Architect"] = cfg.Agents.Testers.Providers[0].Name
	}
	if len(cfg.Agents.QA.Providers) > 0 {
		roles["QA Specialist"] = cfg.Agents.QA.Providers[0].Name
	}
	return roles
}
