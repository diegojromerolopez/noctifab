package services

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsAutoResolvableConflictFile checks if a conflicted file path is a manifest or root export file.
func IsAutoResolvableConflictFile(filePath string) bool {
	base := strings.ToLower(filepath.Base(filePath))
	switch base {
	case "cargo.toml", "go.mod", "package.json", "requirements.txt", "gemfile":
		return true
	case "lib.rs", "index.ts", "index.js", "__init__.py":
		return true
	}
	return strings.HasSuffix(base, ".csproj")
}

// AutoResolveConflictFile parses git conflict markers in filePath and merges by unioning declarations.
func AutoResolveConflictFile(filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, err
	}
	str := string(content)
	if !strings.Contains(str, "<<<<<<<") || !strings.Contains(str, "=======") || !strings.Contains(str, ">>>>>>>") {
		return false, nil
	}

	base := strings.ToLower(filepath.Base(filePath))
	var resolved []byte
	var resolveErr error

	switch {
	case base == "package.json":
		resolved, resolveErr = resolvePackageJSONConflict(str)
	case base == "cargo.toml" || base == "go.mod" || base == "requirements.txt" || base == "gemfile" || strings.HasSuffix(base, ".csproj"):
		resolved, resolveErr = resolveLineUnionConflict(str)
	case base == "lib.rs" || base == "index.ts" || base == "index.js" || base == "__init__.py":
		resolved, resolveErr = resolveLineUnionConflict(str)
	default:
		return false, nil
	}

	if resolveErr != nil || len(resolved) == 0 {
		return false, resolveErr
	}

	if err := os.WriteFile(filePath, resolved, 0644); err != nil {
		return false, err
	}

	fmt.Printf("✨ [Rebase Auto-Resolver] Cleanly auto-resolved manifest/root conflict in %s\n", filePath)
	return true, nil
}

// resolveLineUnionConflict extracts both sides of the conflict block and merges unique non-empty lines.
func resolveLineUnionConflict(content string) ([]byte, error) {
	var out bytes.Buffer
	scanner := bufio.NewScanner(strings.NewReader(content))
	inConflict := false
	inTheirs := false
	var oursLines []string
	var theirsLines []string

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "<<<<<<<") {
			inConflict = true
			inTheirs = false
			oursLines = nil
			theirsLines = nil
			continue
		}
		if strings.HasPrefix(line, "=======") && inConflict {
			inTheirs = true
			continue
		}
		if strings.HasPrefix(line, ">>>>>>>") && inConflict {
			inConflict = false
			// Union merge both sides
			seen := make(map[string]bool)
			for _, l := range oursLines {
				trimmed := strings.TrimSpace(l)
				if trimmed != "" {
					seen[trimmed] = true
				}
				out.WriteString(l + "\n")
			}
			for _, l := range theirsLines {
				trimmed := strings.TrimSpace(l)
				if trimmed == "" || !seen[trimmed] {
					out.WriteString(l + "\n")
					if trimmed != "" {
						seen[trimmed] = true
					}
				}
			}
			continue
		}

		if inConflict {
			if inTheirs {
				theirsLines = append(theirsLines, line)
			} else {
				oursLines = append(oursLines, line)
			}
		} else {
			out.WriteString(line + "\n")
		}
	}

	return out.Bytes(), scanner.Err()
}

// resolvePackageJSONConflict extracts JSON from both sides and merges dependency maps.
func resolvePackageJSONConflict(content string) ([]byte, error) {
	lines, err := resolveLineUnionConflict(content)
	if err != nil {
		return nil, err
	}
	// Verify valid JSON
	var val map[string]interface{}
	if jErr := json.Unmarshal(lines, &val); jErr == nil {
		return json.MarshalIndent(val, "", "  ")
	}
	return lines, nil
}

// TryAutoResolveRebaseConflicts scans git status for conflicted files and resolves all resolvable manifests.
func TryAutoResolveRebaseConflicts(ctx context.Context, git *GitClient) (bool, error) {
	if git == nil {
		return false, nil
	}
	statusOut, err := git.Run(ctx, false, "status", "--porcelain")
	if err != nil {
		return false, err
	}

	allResolved := true
	anyResolved := false

	for _, line := range strings.Split(statusOut, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 4 {
			continue
		}
		// Unmerged files in git status --porcelain have UU, AA, UD, DU, etc.
		statusCode := trimmed[:2]
		filePath := strings.TrimSpace(trimmed[2:])

		if statusCode == "UU" || statusCode == "AA" || statusCode == "UD" || statusCode == "DU" {
			fullPath := filepath.Join(git.Dir(), filePath)
			if IsAutoResolvableConflictFile(filePath) {
				resolved, rErr := AutoResolveConflictFile(fullPath)
				if rErr == nil && resolved {
					_, _ = git.Run(ctx, true, "add", filePath)
					anyResolved = true
				} else {
					allResolved = false
				}
			} else {
				allResolved = false
			}
		}
	}

	return anyResolved && allResolved, nil
}
