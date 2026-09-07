package services

import (
	"fmt"
	"strings"
)

// ApplyFileEdits applies a list of replacement chunks to file content with resilient
// fallback matching (e.g. single-occurrence file-wide search) and clear small-file write_file guidance.
func ApplyFileEdits(content string, edits []ReplacementChunk, relPath string) (string, error) {
	lines := strings.Split(content, "\n")

	for _, edit := range edits {
		start := edit.StartLine
		end := edit.EndLine
		if start < 1 {
			start = 1
		}
		if end > len(lines) {
			end = len(lines)
		}
		if start > end {
			return "", fmt.Errorf("invalid line range %d-%d", start, end)
		}

		// Slice lines is 0-indexed, start/end are 1-indexed
		targetSlice := lines[start-1 : end]
		targetJoined := strings.Join(targetSlice, "\n")

		if strings.Contains(targetJoined, edit.TargetContent) {
			replacedJoined := strings.Replace(targetJoined, edit.TargetContent, edit.ReplacementContent, 1)
			replacedLines := strings.Split(replacedJoined, "\n")

			newLines := append([]string{}, lines[:start-1]...)
			newLines = append(newLines, replacedLines...)
			newLines = append(newLines, lines[end:]...)
			lines = newLines
			continue
		}

		// Resilient Fallback 1: Check if target_content exists exactly once in the entire file
		currentJoined := strings.Join(lines, "\n")
		if strings.Count(currentJoined, edit.TargetContent) == 1 {
			currentJoined = strings.Replace(currentJoined, edit.TargetContent, edit.ReplacementContent, 1)
			lines = strings.Split(currentJoined, "\n")
			continue
		}

		// Resilient Fallback 2: Check with whitespace trimming if single match exists
		trimmedTarget := strings.TrimSpace(edit.TargetContent)
		if trimmedTarget != "" && strings.Count(currentJoined, trimmedTarget) == 1 {
			currentJoined = strings.Replace(currentJoined, trimmedTarget, strings.TrimSpace(edit.ReplacementContent), 1)
			lines = strings.Split(currentJoined, "\n")
			continue
		}

		// Failure reporting: if file is <= 300 lines, guide agent directly to write_file
		if len(lines) <= 300 {
			return "", fmt.Errorf(
				"edit_file failed: target_content not found in %s (%d lines). "+
					"Because the file is small (<= 300 lines), DO NOT retry edit_file: "+
					"call 'write_file' with path='%s' and content='<full file content>' to replace the file completely and eliminate line matching errors",
				relPath, len(lines), relPath,
			)
		}

		return "", fmt.Errorf(
			"edit_file failed: target_content not found in file (range %d-%d). "+
				"The file content may have changed since you last read it. "+
				"Call read_file first to get current content, then retry edit_file with exact matching target_content, "+
				"or use write_file to overwrite the entire file with corrected content",
			edit.StartLine, edit.EndLine,
		)
	}

	return strings.Join(lines, "\n"), nil
}
