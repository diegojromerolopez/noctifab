package services

import (
	"context"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SyntaxViolation represents a syntax or parse error detected in a file before tests are run.
type SyntaxViolation struct {
	FilePath string
	Line     int
	Message  string
}

// SyntaxValidator provides fast, in-process and deterministic syntax verification
// across supported source code and structured configuration formats.
type SyntaxValidator struct{}

// NewSyntaxValidator creates a new SyntaxValidator instance.
func NewSyntaxValidator() *SyntaxValidator {
	return &SyntaxValidator{}
}

// ValidateFile performs deterministic syntax validation on a single file based on its extension.
func (v *SyntaxValidator) ValidateFile(ctx context.Context, fullPath string) (*SyntaxViolation, error) {
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // File was deleted or does not exist
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, nil
	}

	ext := strings.ToLower(filepath.Ext(fullPath))
	switch ext {
	case ".go":
		return v.validateGo(fullPath)
	case ".json":
		return v.validateJSON(fullPath)
	case ".py":
		return v.validatePython(ctx, fullPath)
	default:
		return nil, nil
	}
}

// ValidateFiles validates a slice of file paths (either absolute or relative to baseDir).
func (v *SyntaxValidator) ValidateFiles(ctx context.Context, baseDir string, files []string) ([]SyntaxViolation, error) {
	var violations []SyntaxViolation
	for _, rel := range files {
		fullPath := rel
		if !filepath.IsAbs(fullPath) && baseDir != "" {
			fullPath = filepath.Join(baseDir, fullPath)
		}
		violation, err := v.ValidateFile(ctx, fullPath)
		if err != nil {
			return nil, err
		}
		if violation != nil {
			violations = append(violations, *violation)
		}
	}
	return violations, nil
}

func (v *SyntaxValidator) validateGo(path string) (*SyntaxViolation, error) {
	fset := token.NewFileSet()
	_, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
	if err != nil {
		return &SyntaxViolation{
			FilePath: path,
			Message:  fmt.Sprintf("Go syntax error: %v", err),
		}, nil
	}
	return nil, nil
}

func (v *SyntaxValidator) validateJSON(path string) (*SyntaxViolation, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(content))) == 0 {
		return nil, nil
	}
	var dummy any
	if err := json.Unmarshal(content, &dummy); err != nil {
		return &SyntaxViolation{
			FilePath: path,
			Message:  fmt.Sprintf("JSON syntax error: %v", err),
		}, nil
	}
	return nil, nil
}

func (v *SyntaxValidator) validatePython(ctx context.Context, path string) (*SyntaxViolation, error) {
	// Attempt fast Python compilation via python3 -m py_compile if python3 is available
	pythonPath, err := exec.LookPath("python3")
	if err != nil {
		return nil, nil // python3 not on host path, skip pre-check
	}

	cmd := exec.CommandContext(ctx, pythonPath, "-m", "py_compile", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return &SyntaxViolation{
			FilePath: path,
			Message:  fmt.Sprintf("Python syntax error: %s", msg),
		}, nil
	}
	return nil, nil
}
