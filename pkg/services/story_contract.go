package services

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

var (
	storyContractBlockRE = regexp.MustCompile("(?s)```noctifab-contract[ \\t]*\\r?\\n(.*?)\\r?\\n```")
	contractIDRE         = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	windowsAbsoluteRE    = regexp.MustCompile(`^[A-Za-z]:[/\\]`)
)

type storyContractPayload struct {
	StoryID         string                  `json:"story_id"`
	PublicContracts []domain.PublicContract `json:"public_contracts"`
}

// ParseStoryContract extracts and validates the machine-readable contract from a roadmap story.
func ParseStoryContract(sourcePath, markdown string) (domain.StoryContract, error) {
	matches := storyContractBlockRE.FindAllStringSubmatch(markdown, -1)
	if len(matches) != 1 {
		return domain.StoryContract{}, storyContractError("expected exactly one noctifab-contract block")
	}

	var payload storyContractPayload
	decoder := json.NewDecoder(strings.NewReader(matches[0][1]))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return domain.StoryContract{}, storyContractError("invalid JSON: %v", err)
	}
	if err := requireJSONEOF(decoder); err != nil {
		return domain.StoryContract{}, storyContractError("invalid JSON: %v", err)
	}

	contract := domain.StoryContract{
		StoryID:         strings.TrimSpace(payload.StoryID),
		PublicContracts: payload.PublicContracts,
	}
	if contract.StoryID == "" {
		return domain.StoryContract{}, storyContractError("story_id is required")
	}
	if len(contract.PublicContracts) == 0 {
		return domain.StoryContract{}, storyContractError("at least one public contract is required")
	}

	seen := make(map[string]struct{}, len(contract.PublicContracts))
	for i := range contract.PublicContracts {
		publicContract := &contract.PublicContracts[i]
		publicContract.ID = strings.TrimSpace(publicContract.ID)
		if !contractIDRE.MatchString(publicContract.ID) {
			return domain.StoryContract{}, storyContractError("public_contracts[%d].id is invalid", i)
		}
		if _, exists := seen[publicContract.ID]; exists {
			return domain.StoryContract{}, storyContractError("duplicate public contract id %q", publicContract.ID)
		}
		seen[publicContract.ID] = struct{}{}

		var err error
		publicContract.ApplicablePathPrefixes, err = normalizeRelativePaths(publicContract.ApplicablePathPrefixes, false)
		if err != nil {
			return domain.StoryContract{}, storyContractError("public contract %q applicable_path_prefixes: %v", publicContract.ID, err)
		}
		publicContract.AllowedExecutables, err = normalizeRelativePaths(publicContract.AllowedExecutables, true)
		if err != nil {
			return domain.StoryContract{}, storyContractError("public contract %q allowed_executables: %v", publicContract.ID, err)
		}
		if len(publicContract.AllowedExecutables) == 0 {
			return domain.StoryContract{}, storyContractError("public contract %q requires an allowed executable", publicContract.ID)
		}
		if len(publicContract.ExitCodes) == 0 && len(publicContract.StdoutContains) == 0 && len(publicContract.StderrPrefixes) == 0 {
			return domain.StoryContract{}, storyContractError("public contract %q has no observable expectation", publicContract.ID)
		}
	}

	cleanSource, err := normalizeRelativePath(sourcePath, false)
	if err != nil {
		return domain.StoryContract{}, storyContractError("source path: %v", err)
	}
	sum := sha256.Sum256([]byte(markdown))
	contract.SourcePath = cleanSource
	contract.SourceSHA256 = hex.EncodeToString(sum[:])
	return contract, nil
}

func normalizeRelativePaths(values []string, executable bool) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	normalized := make([]string, len(values))
	for i, value := range values {
		clean, err := normalizeRelativePath(value, executable)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		normalized[i] = clean
	}
	return normalized, nil
}

func normalizeRelativePath(value string, executable bool) (string, error) {
	raw := strings.TrimSpace(strings.ReplaceAll(value, `\`, "/"))
	if raw == "" {
		return "", fmt.Errorf("path is empty")
	}
	if path.IsAbs(raw) || windowsAbsoluteRE.MatchString(raw) {
		return "", fmt.Errorf("path %q must be repository-relative", value)
	}
	for _, part := range strings.Split(raw, "/") {
		if part == ".." {
			return "", fmt.Errorf("path %q must not contain .. segments", value)
		}
	}
	clean := path.Clean(raw)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("path %q is not a file or directory path", value)
	}
	if executable && strings.HasPrefix(raw, "./") {
		clean = "./" + clean
	}
	return clean, nil
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("multiple JSON values")
	} else if !strings.Contains(err.Error(), "EOF") {
		return err
	}
	return nil
}

func storyContractError(format string, args ...any) error {
	var message bytes.Buffer
	_, _ = fmt.Fprintf(&message, format, args...)
	return fmt.Errorf("story contract: %s", message.String())
}

// FormatContractPromptContext renders a machine-readable summary of public contract expectations
// for injection into Generator and Tester agent prompts.
func FormatContractPromptContext(sourcePath, markdown string) string {
	contract, err := ParseStoryContract(sourcePath, markdown)
	if err != nil || len(contract.PublicContracts) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("### BLACK-BOX CONTRACT EXPECTATIONS (NON-NEGOTIABLE)\n\n")
	sb.WriteString("The implementation MUST satisfy the following public observable expectations:\n\n")

	for _, pc := range contract.PublicContracts {
		fmt.Fprintf(&sb, "- **Contract ID:** `%s`\n", pc.ID)
		if len(pc.AllowedExecutables) > 0 {
			fmt.Fprintf(&sb, "  - **Allowed Executables:** `%s`\n", strings.Join(pc.AllowedExecutables, "`, `"))
		}
		if len(pc.ExitCodes) > 0 {
			fmt.Fprintf(&sb, "  - **Expected Exit Codes:** `%v`\n", pc.ExitCodes)
		}
		if len(pc.StderrPrefixes) > 0 {
			fmt.Fprintf(&sb, "  - **Expected Stderr Prefixes:** `%q`\n", pc.StderrPrefixes)
		}
		if len(pc.StdoutContains) > 0 {
			fmt.Fprintf(&sb, "  - **Expected Stdout Substrings:** `%q`\n", pc.StdoutContains)
		}
	}

	return sb.String()
}
