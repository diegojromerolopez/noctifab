package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SpecManifest describes the deterministic partitioning of SPEC.md into .noctifab/specs/.
type SpecManifest struct {
	SpecSource  string             `json:"spec_source"`
	SpecSHA256  string             `json:"spec_sha256"`
	GeneratedAt string             `json:"generated_at"`
	TotalLines  int                `json:"total_lines"`
	TotalBytes  int                `json:"total_bytes"`
	CoreFile    string             `json:"core_file"`
	Sections    []SpecSectionEntry `json:"sections"`
}

// SpecSectionEntry represents a sliced domain subsystem from SPEC.md.
type SpecSectionEntry struct {
	ID              string   `json:"id"`
	File            string   `json:"file"`
	Title           string   `json:"title"`
	HeadingLevel    int      `json:"heading_level"`
	LineStart       int      `json:"line_start"`
	LineEnd         int      `json:"line_end"`
	CommandCount    int      `json:"command_count,omitempty"`
	Commands        []string `json:"commands,omitempty"`
	SuggestedTarget string   `json:"suggested_target,omitempty"`
}

var commandTableRowRegex = regexp.MustCompile(`^\|\s*` + "`([A-Z0-9_]+)`" + `\s*\|`)

// PartitionSpecIfNeeded checks if projectPath/SPEC.md exists and partitions it into .noctifab/specs/.
// It is idempotent and skips re-partitioning if the SHA256 matches manifest.json.
func PartitionSpecIfNeeded(projectPath string) (*SpecManifest, error) {
	specPath := filepath.Join(projectPath, "SPEC.md")
	data, err := os.ReadFile(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading SPEC.md: %w", err)
	}

	specsDir := filepath.Join(projectPath, ".noctifab", "specs")
	manifestPath := filepath.Join(specsDir, "manifest.json")

	hash := sha256.Sum256(data)
	specHash := hex.EncodeToString(hash[:])

	// Check cache
	if mData, err := os.ReadFile(manifestPath); err == nil {
		var manifest SpecManifest
		if json.Unmarshal(mData, &manifest) == nil && manifest.SpecSHA256 == specHash {
			return &manifest, nil
		}
	}

	return PartitionSpec(string(data), specsDir, specHash)
}

// PartitionSpec deterministically parses specContent and writes slices + manifest.json into outputDir.
func PartitionSpec(specContent string, outputDir string, specHash string) (*SpecManifest, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating specs output directory %q: %w", outputDir, err)
	}

	lines := strings.Split(specContent, "\n")
	manifest := &SpecManifest{
		SpecSource:  "SPEC.md",
		SpecSHA256:  specHash,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		TotalLines:  len(lines),
		TotalBytes:  len(specContent),
		CoreFile:    "00_core_invariants.md",
		Sections:    make([]SpecSectionEntry, 0),
	}

	type sectionBlock struct {
		title    string
		level    int
		start    int
		end      int
		lines    []string
		commands []string
	}

	var parsedSections []sectionBlock
	current := sectionBlock{start: 0}

	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") || strings.HasPrefix(trimmed, "### ") {
			if len(current.lines) > 0 || current.title != "" {
				current.end = idx
				parsedSections = append(parsedSections, current)
			}
			level := 2
			if strings.HasPrefix(trimmed, "### ") {
				level = 3
			}
			cleanTitle := strings.TrimSpace(strings.TrimLeft(trimmed, "# "))
			current = sectionBlock{
				title: cleanTitle,
				level: level,
				start: idx + 1,
				lines: []string{line},
			}
			continue
		}

		if match := commandTableRowRegex.FindStringSubmatch(trimmed); len(match) > 1 {
			current.commands = append(current.commands, match[1])
		}
		current.lines = append(current.lines, line)
	}

	if len(current.lines) > 0 {
		current.end = len(lines)
		parsedSections = append(parsedSections, current)
	}

	var coreLines []string
	sliceIndex := 1

	for _, sec := range parsedSections {
		// If a section has commands or is a detailed sub-heading under features (e.g. 6.X or level 3 command group)
		isDomainSlice := len(sec.commands) > 0 || (sec.level == 3 && strings.Contains(strings.ToLower(sec.title), "command"))

		if isDomainSlice {
			slug := sanitizeSlug(sec.title)
			fileName := fmt.Sprintf("%02d_%s.md", sliceIndex, slug)
			filePath := filepath.Join(outputDir, fileName)

			sliceContent := strings.Join(sec.lines, "\n")
			if err := os.WriteFile(filePath, []byte(sliceContent), 0644); err != nil {
				return nil, fmt.Errorf("writing domain slice %q: %w", fileName, err)
			}

			suggestedTarget := deriveSuggestedTarget(slug)
			entry := SpecSectionEntry{
				ID:              slug,
				File:            fileName,
				Title:           sec.title,
				HeadingLevel:    sec.level,
				LineStart:       sec.start,
				LineEnd:         sec.end,
				CommandCount:    len(sec.commands),
				Commands:        sec.commands,
				SuggestedTarget: suggestedTarget,
			}
			manifest.Sections = append(manifest.Sections, entry)
			sliceIndex++

			// In core invariants, add a reference link rather than copying the entire domain table
			coreLines = append(coreLines, fmt.Sprintf("### %s\n*(Detailed command matrix partitioned into `.noctifab/specs/%s` — %d commands)*\n", sec.title, fileName, len(sec.commands)))
		} else {
			coreLines = append(coreLines, sec.lines...)
		}
	}

	// Write 00_core_invariants.md
	coreFilePath := filepath.Join(outputDir, manifest.CoreFile)
	coreContent := strings.Join(coreLines, "\n")
	if err := os.WriteFile(coreFilePath, []byte(coreContent), 0644); err != nil {
		return nil, fmt.Errorf("writing core invariants file %q: %w", manifest.CoreFile, err)
	}

	// Write manifest.json
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshalling spec manifest: %w", err)
	}
	manifestPath := filepath.Join(outputDir, "manifest.json")
	if err := os.WriteFile(manifestPath, manifestBytes, 0644); err != nil {
		return nil, fmt.Errorf("writing manifest.json: %w", err)
	}

	return manifest, nil
}

func sanitizeSlug(title string) string {
	lower := strings.ToLower(title)
	var sb strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else if sb.Len() > 0 && sb.String()[sb.Len()-1] != '_' {
			sb.WriteRune('_')
		}
	}
	res := strings.Trim(sb.String(), "_")
	// Trim common numbering prefixes like "6_1_" or "6_10_"
	numPrefixRegex := regexp.MustCompile(`^[0-9]+_[0-9]+_`)
	res = numPrefixRegex.ReplaceAllString(res, "")
	res = strings.TrimSuffix(res, "_commands")
	if res == "" {
		return "subsystem"
	}
	return res
}

func deriveSuggestedTarget(slug string) string {
	switch {
	case strings.Contains(slug, "string"):
		return "src/commands/strings.py"
	case strings.Contains(slug, "list"):
		return "src/commands/lists.py"
	case strings.Contains(slug, "hash"):
		return "src/commands/hashes.py"
	case strings.Contains(slug, "set") && !strings.Contains(slug, "zset"):
		return "src/commands/sets.py"
	case strings.Contains(slug, "zset") || strings.Contains(slug, "sorted"):
		return "src/commands/zsets.py"
	case strings.Contains(slug, "stream"):
		return "src/commands/streams.py"
	case strings.Contains(slug, "transact") || strings.Contains(slug, "multi"):
		return "src/commands/multi.py"
	case strings.Contains(slug, "pubsub"):
		return "src/commands/pubsub.py"
	case strings.Contains(slug, "generic") || strings.Contains(slug, "key"):
		return "src/commands/generic.py"
	case strings.Contains(slug, "server"):
		return "src/commands/server.py"
	case strings.Contains(slug, "bit"):
		return "src/commands/bitmaps.py"
	default:
		return fmt.Sprintf("src/commands/%s.py", slug)
	}
}
