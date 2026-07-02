package usecase

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/trace"
)

var semverRegex = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

// BumpVersion reads the VERSION file, parses it, determines the bump type based on completed tasks,
// increments the semver string, and writes it back.
func BumpVersion(projectPath string, tasks []domain.Task) (string, error) {
	_, span := telemetry.Tracer().Start(context.Background(), "BumpVersion",
		trace.WithAttributes(telemetry.Attr("project_path", projectPath)))
	defer span.End()
	vPath := filepath.Join(projectPath, "VERSION")
	contentBytes, err := os.ReadFile(vPath)
	var current string
	if err != nil {
		if os.IsNotExist(err) {
			current = "0.0.1"
		} else {
			return "", err
		}
	} else {
		current = strings.TrimSpace(string(contentBytes))
	}

	matches := semverRegex.FindStringSubmatch(current)
	if len(matches) != 4 {
		return "", fmt.Errorf("invalid semver version in VERSION file: '%s'", current)
	}

	major, _ := strconv.Atoi(matches[1])
	minor, _ := strconv.Atoi(matches[2])
	patch, _ := strconv.Atoi(matches[3])

	// Aggregate change types
	hasBreaking := false
	hasFeature := false
	for _, t := range tasks {
		if t.Status == domain.TaskSuccess {
			switch t.ChangeType {
			case domain.ChangeTypeBreaking:
				hasBreaking = true
			case domain.ChangeTypeFeature:
				hasFeature = true
			}
		}
	}

	if hasBreaking {
		major++
		minor = 0
		patch = 0
	} else if hasFeature {
		minor++
		patch = 0
	} else {
		patch++
	}

	nextVersion := fmt.Sprintf("%d.%d.%d", major, minor, patch)
	if err := os.WriteFile(vPath, []byte(nextVersion+"\n"), 0644); err != nil {
		return "", err
	}

	return nextVersion, nil
}

// UpdateChangelog aggregates task partial changelogs and prepends them to CHANGELOG.md under `# Changelog`.
func UpdateChangelog(projectPath string, version string, tasks []domain.Task) error {
	cPath := filepath.Join(projectPath, "CHANGELOG.md")
	var existingContent string
	contentBytes, err := os.ReadFile(cPath)
	if err == nil {
		existingContent = string(contentBytes)
	}

	// Categorize change descriptions
	added := []string{}
	fixed := []string{}
	changed := []string{}

	for _, t := range tasks {
		if t.Status == domain.TaskSuccess {
			for _, desc := range t.PartialChangelog {
				cleaned := strings.TrimSpace(desc)
				if cleaned == "" {
					continue
				}
				// Basic categorization based on keywords
				lower := strings.ToLower(cleaned)
				if strings.HasPrefix(lower, "fix") || strings.Contains(lower, "bug") {
					fixed = append(fixed, cleaned)
				} else if strings.HasPrefix(lower, "add") || strings.HasPrefix(lower, "implement") {
					added = append(added, cleaned)
				} else {
					changed = append(changed, cleaned)
				}
			}
		}
	}

	// Format release entry
	var sb strings.Builder
	dateStr := time.Now().Format("2006-01-02")
	fmt.Fprintf(&sb, "\n## [%s] - %s\n", version, dateStr)

	if len(added) > 0 {
		sb.WriteString("### Added\n")
		for _, item := range added {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
	}
	if len(changed) > 0 {
		sb.WriteString("### Changed\n")
		for _, item := range changed {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
	}
	if len(fixed) > 0 {
		sb.WriteString("### Fixed\n")
		for _, item := range fixed {
			fmt.Fprintf(&sb, "- %s\n", item)
		}
	}

	newEntry := sb.String()

	var finalChangelog string
	if existingContent == "" {
		finalChangelog = "# Changelog\n" + newEntry
	} else {
		// Insert right after `# Changelog` heading
		headingIndex := strings.Index(existingContent, "# Changelog")
		if headingIndex == -1 {
			finalChangelog = "# Changelog\n" + newEntry + "\n" + existingContent
		} else {
			insertAt := headingIndex + len("# Changelog")
			// Skip trailing newlines/spaces after header
			for insertAt < len(existingContent) && (existingContent[insertAt] == '\n' || existingContent[insertAt] == '\r') {
				insertAt++
			}
			finalChangelog = existingContent[:insertAt] + newEntry + "\n" + existingContent[insertAt:]
		}
	}

	return os.WriteFile(cPath, []byte(finalChangelog), 0644)
}
