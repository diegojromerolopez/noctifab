package templates

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"gopkg.in/yaml.v3"
)

// Manager resolves, lists, and applies language templates across workspace, global, and built-in tiers.
type Manager struct {
	workspaceDir string
	globalDir    string
}

// NewManager creates a template manager for the given workspace directory.
func NewManager(workspaceDir string) *Manager {
	globalDir := ""
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		globalDir = filepath.Join(home, ".noctifab", "templates")
	}
	return &Manager{
		workspaceDir: workspaceDir,
		globalDir:    globalDir,
	}
}

// SetGlobalDir overrides the global template directory (useful for hermetic unit testing).
func (m *Manager) SetGlobalDir(dir string) {
	m.globalDir = dir
}

// GetWorkspaceDir returns the workspace template directory path.
func (m *Manager) GetWorkspaceDir() string {
	if m.workspaceDir == "" {
		return filepath.Join(".noctifab", "templates")
	}
	return filepath.Join(m.workspaceDir, ".noctifab", "templates")
}

// GetGlobalDir returns the user-level global template directory path.
func (m *Manager) GetGlobalDir() string {
	return m.globalDir
}

// Get resolves a template by name, inspecting workspace, then global, then built-in tiers.
func (m *Manager) Get(name string) (content string, source TemplateSource, path string, err error) {
	canonical := CanonicalTemplateName(strings.ToLower(strings.TrimSpace(name)))
	if canonical == "" {
		return "", "", "", fmt.Errorf("template name cannot be empty")
	}

	// 1. Check workspace tier: <workspace>/.noctifab/templates/<name>.yaml or .yml
	wsDir := m.GetWorkspaceDir()
	if wsDir != "" {
		for _, ext := range []string{".yaml", ".yml"} {
			candidate := filepath.Join(wsDir, canonical+ext)
			if data, err := os.ReadFile(candidate); err == nil {
				return string(data), SourceWorkspace, candidate, nil
			}
		}
	}

	// 2. Check global tier: ~/.noctifab/templates/<name>.yaml or .yml
	if m.globalDir != "" {
		for _, ext := range []string{".yaml", ".yml"} {
			candidate := filepath.Join(m.globalDir, canonical+ext)
			if data, err := os.ReadFile(candidate); err == nil {
				return string(data), SourceGlobal, candidate, nil
			}
		}
	}

	// 3. Check embedded built-in tier
	if builtin, ok := BuiltinTemplates[canonical]; ok {
		return builtin.YAML, SourceBuiltin, "", nil
	}

	return "", "", "", fmt.Errorf("template %q not found in workspace, global, or built-in templates", name)
}

// List returns all discovered templates across workspace, global, and built-in tiers.
func (m *Manager) List() ([]TemplateInfo, error) {
	discovered := make(map[string]TemplateInfo)

	// 1. Builtin defaults
	for name, b := range BuiltinTemplates {
		discovered[name] = TemplateInfo{
			Name:        name,
			Language:    name,
			Description: b.Description,
			Source:      SourceBuiltin,
		}
	}

	// 2. Global templates (~/.noctifab/templates/)
	if m.globalDir != "" {
		if entries, err := os.ReadDir(m.globalDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if ext == ".yaml" || ext == ".yml" {
					base := strings.TrimSuffix(entry.Name(), ext)
					canonical := CanonicalTemplateName(strings.ToLower(base))
					discovered[canonical] = TemplateInfo{
						Name:        canonical,
						Language:    canonical,
						Description: fmt.Sprintf("User global template (%s)", entry.Name()),
						Source:      SourceGlobal,
						Path:        filepath.Join(m.globalDir, entry.Name()),
					}
				}
			}
		}
	}

	// 3. Workspace templates (<workspace>/.noctifab/templates/)
	wsDir := m.GetWorkspaceDir()
	if wsDir != "" {
		if entries, err := os.ReadDir(wsDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				if ext == ".yaml" || ext == ".yml" {
					base := strings.TrimSuffix(entry.Name(), ext)
					canonical := CanonicalTemplateName(strings.ToLower(base))
					discovered[canonical] = TemplateInfo{
						Name:        canonical,
						Language:    canonical,
						Description: fmt.Sprintf("Workspace template (%s)", entry.Name()),
						Source:      SourceWorkspace,
						Path:        filepath.Join(wsDir, entry.Name()),
					}
				}
			}
		}
	}

	var list []TemplateInfo
	for _, info := range discovered {
		list = append(list, info)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list, nil
}

// Apply resolves and overlays the template configuration onto the provided Config struct.
func (m *Manager) Apply(cfg *config.Config, name string) error {
	content, _, _, err := m.Get(name)
	if err != nil {
		return err
	}

	decoder := yaml.NewDecoder(strings.NewReader(content))
	if err := decoder.Decode(cfg); err != nil {
		return fmt.Errorf("failed to decode template %q onto config: %w", name, err)
	}
	return nil
}

// Export writes the specified template into the destination directory as a YAML file.
func (m *Manager) Export(name string, destDir string, overwrite bool) (string, error) {
	canonical := CanonicalTemplateName(strings.ToLower(strings.TrimSpace(name)))
	content, _, _, err := m.Get(canonical)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create destination directory: %w", err)
	}

	destPath := filepath.Join(destDir, canonical+".yaml")
	if !overwrite {
		if _, err := os.Stat(destPath); err == nil {
			return destPath, fmt.Errorf("template file %s already exists", destPath)
		}
	}

	if err := os.WriteFile(destPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write template to %s: %w", destPath, err)
	}
	return destPath, nil
}
