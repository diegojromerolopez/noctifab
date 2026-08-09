package prompts

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed defaults/*/*.tmpl
var defaultsFS embed.FS

// Source identifies where the effective template for a key came from.
type Source string

const (
	// SourceConfig means an explicit path override from config.yaml.
	SourceConfig Source = "config"
	// SourceConvention means an auto-discovered workspace file
	// (.noctifab/prompts/<agent>/<action>.tmpl).
	SourceConvention Source = "convention"
	// SourceEmbedded means the default template shipped inside the binary.
	SourceEmbedded Source = "embedded"
)

// Override is a per-(agent, action) customization from configuration:
// an explicit full-template path and/or an append string.
type Override struct {
	// Path is an explicit template file path (absolute, or relative to the
	// workspace directory). It replaces the whole default body.
	Path string
	// Append is appended verbatim to the end of the DEFAULT body. Ignored
	// (with a warning) when a full-template override is active.
	Append string
}

// resolved is the outcome of resolving one (agent, action) key.
type resolved struct {
	// text is the effective template body text (append already applied).
	text string
	// source is where the body came from.
	source Source
	// appendSource describes the applied append origin: "config", "convention"
	// or "" when no append is in effect.
	appendSource string
}

// conventionDir returns the workspace convention directory for prompts.
func conventionDir(workspaceDir string) string {
	return filepath.Join(workspaceDir, ".noctifab", "prompts")
}

// conventionPath returns the convention override path for a key.
func conventionPath(workspaceDir, agent, action string) string {
	return filepath.Join(conventionDir(workspaceDir), agent, action+".tmpl")
}

// conventionAppendPath returns the convention append path for a key.
func conventionAppendPath(workspaceDir, agent, action string) string {
	return filepath.Join(conventionDir(workspaceDir), agent, action+".append.tmpl")
}

// appendConflictError builds the fail-fast error for the forbidden
// combination of a full-template override and an append on the same action.
// Appends apply to the DEFAULT body only; silently ignoring one of two
// explicit user opt-ins would mask a configuration mistake.
func appendConflictError(agent, action, appendSource, override string) error {
	return fmt.Errorf(
		"prompt configuration conflict for %s/%s: a full-template override (%s) and an append (%s) are both configured; appends apply to the default body only — remove one of the two",
		agent, action, override, appendSource)
}

// DefaultTemplate returns the embedded default template body for a key.
func DefaultTemplate(agent, action string) (string, error) {
	if err := ValidateKey(agent, action); err != nil {
		return "", err
	}
	data, err := defaultsFS.ReadFile("defaults/" + agent + "/" + action + ".tmpl")
	if err != nil {
		return "", fmt.Errorf("missing embedded default template for %s/%s: %w", agent, action, err)
	}
	return string(data), nil
}

// resolveKey resolves the effective template body for one (agent, action)
// key: config path > convention file > embedded default. Appends (config
// string > convention append file) apply to the embedded default body only.
func resolveKey(workspaceDir, agent, action string, ov Override, warn func(format string, args ...any)) (resolved, error) {
	def, err := DefaultTemplate(agent, action)
	if err != nil {
		return resolved{}, err
	}

	// Resolve the append content first (config string wins over file).
	appendText := ""
	appendSource := ""
	if ov.Append != "" {
		appendText = ov.Append
		appendSource = "config"
	}
	appendFile := conventionAppendPath(workspaceDir, agent, action)
	if fileData, fErr := os.ReadFile(appendFile); fErr == nil {
		if appendSource == "config" {
			warn("prompts: %s/%s has both a config 'append' and %s; the config append wins and the file is ignored", agent, action, appendFile)
		} else {
			appendText = string(fileData)
			appendSource = "convention"
		}
	}

	// 1. Explicit config path override.
	if ov.Path != "" {
		if appendSource != "" {
			return resolved{}, appendConflictError(agent, action, appendSource, "config path override")
		}
		path := ov.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(workspaceDir, path)
		}
		data, rErr := os.ReadFile(path)
		if rErr != nil {
			return resolved{}, fmt.Errorf("prompt override for %s/%s: cannot read template path %q: %w", agent, action, ov.Path, rErr)
		}
		return resolved{text: string(data), source: SourceConfig}, nil
	}

	// 2. Convention file override.
	convPath := conventionPath(workspaceDir, agent, action)
	if data, rErr := os.ReadFile(convPath); rErr == nil {
		if appendSource != "" {
			return resolved{}, appendConflictError(agent, action, appendSource, "convention file "+convPath)
		}
		return resolved{text: string(data), source: SourceConvention}, nil
	}

	// 3. Embedded default (+ optional append).
	res := resolved{text: def, source: SourceEmbedded, appendSource: appendSource}
	if appendSource != "" {
		res.text = def + appendText
	}
	return res, nil
}
