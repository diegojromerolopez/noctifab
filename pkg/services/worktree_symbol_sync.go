package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var (
	csMissingSymbolRegex   = regexp.MustCompile(`error CS0103: The name '([A-Za-z0-9_]+)' does not exist`)
	goMissingSymbolRegex   = regexp.MustCompile(`undefined:\s+([A-Za-z0-9_]+)`)
	rustMissingSymbolRegex = regexp.MustCompile(`cannot find (?:type|value|function|struct)\s+` + "`" + `([A-Za-z0-9_]+)` + "`")
	pyMissingSymbolRegex   = regexp.MustCompile(`(?:NameError: name '([A-Za-z0-9_]+)' is not defined|ModuleNotFoundError: No module named '([A-Za-z0-9_]+)')`)
	cMissingSymbolRegex    = regexp.MustCompile(`(?:unknown type name '([A-Za-z0-9_]+)'|undeclared identifier '([A-Za-z0-9_]+)')`)
)

// ExtractUnresolvedSymbol parses compiler diagnostic output for missing symbol identifiers.
func ExtractUnresolvedSymbol(compilerOutput string) string {
	if m := csMissingSymbolRegex.FindStringSubmatch(compilerOutput); len(m) > 1 {
		return m[1]
	}
	if m := goMissingSymbolRegex.FindStringSubmatch(compilerOutput); len(m) > 1 {
		return m[1]
	}
	if m := rustMissingSymbolRegex.FindStringSubmatch(compilerOutput); len(m) > 1 {
		return m[1]
	}
	if m := pyMissingSymbolRegex.FindStringSubmatch(compilerOutput); len(m) > 1 {
		for _, sym := range m[1:] {
			if sym != "" {
				return sym
			}
		}
	}
	if m := cMissingSymbolRegex.FindStringSubmatch(compilerOutput); len(m) > 1 {
		for _, sym := range m[1:] {
			if sym != "" {
				return sym
			}
		}
	}
	return ""
}

// AutoLinkSymbolDependencies pre-flights planned tasks. If Task B explicitly references
// a symbol/type produced by Task A, it automatically adds Task A to Task B's DependsOn.
func AutoLinkSymbolDependencies(tasks []domain.Task) {
	entityProducer := make(map[string]string) // entityName -> taskID
	for _, t := range tasks {
		for _, f := range t.TargetFiles {
			base := filepath.Base(f)
			ext := filepath.Ext(base)
			entityName := strings.TrimSuffix(base, ext)
			if len(entityName) >= 3 {
				entityProducer[strings.ToLower(entityName)] = t.ID
			}
		}
	}

	for i := range tasks {
		task := &tasks[i]
		descLower := strings.ToLower(task.Title + " " + task.Description)
		for entity, producerID := range entityProducer {
			if producerID == task.ID {
				continue
			}
			if strings.Contains(descLower, entity) {
				alreadyDep := false
				for _, d := range task.DependsOn {
					if d == producerID {
						alreadyDep = true
						break
					}
				}
				if !alreadyDep {
					task.DependsOn = append(task.DependsOn, producerID)
					fmt.Printf("ℹ [Preflight Dependency] Linked task %s -> %s based on symbol reference %q\n", task.ID, producerID, entity)
				}
			}
		}
	}
}

// SynchronizeMissingSymbolFromSiblings searches other worktrees for the file declaring symbol,
// and copies it into currentWorktree to unblock compilation.
func SynchronizeMissingSymbolFromSiblings(ctx context.Context, currentWorktree, rootProjectPath, symbol string) bool {
	if symbol == "" {
		return false
	}

	worktreesDir := filepath.Join(rootProjectPath, ".noctifab", "worktrees")
	entries, err := os.ReadDir(worktreesDir)
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		siblingPath := filepath.Join(worktreesDir, entry.Name())
		if filepath.Clean(siblingPath) == filepath.Clean(currentWorktree) {
			continue
		}

		// Search for source files in sibling worktree defining the symbol
		var foundRelPath string
		_ = filepath.Walk(siblingPath, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil || info.IsDir() {
				return nil
			}
			if strings.Contains(path, ".git") || strings.Contains(path, "node_modules") || strings.Contains(path, "target") {
				return nil
			}
			base := filepath.Base(path)
			ext := strings.ToLower(filepath.Ext(base))
			nameWithoutExt := strings.TrimSuffix(base, ext)

			if strings.EqualFold(nameWithoutExt, symbol) {
				rel, rErr := filepath.Rel(siblingPath, path)
				if rErr == nil {
					foundRelPath = rel
					return filepath.SkipAll
				}
			}
			return nil
		})

		if foundRelPath != "" {
			src := filepath.Join(siblingPath, foundRelPath)
			dst := filepath.Join(currentWorktree, foundRelPath)
			if _, statDst := os.Stat(dst); statDst != nil {
				_ = os.MkdirAll(filepath.Dir(dst), 0755)
				data, rErr := os.ReadFile(src)
				if rErr == nil {
					if wErr := os.WriteFile(dst, data, 0644); wErr == nil {
						fmt.Printf("✨ [Symbol Synchronization] Resolved missing symbol %q from sibling %s. Materialized %s\n", symbol, entry.Name(), foundRelPath)
						return true
					}
				}
			}
		}
	}

	return false
}
