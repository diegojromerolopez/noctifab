package services

import (
	"fmt"
	"strings"
)

// ExtractEditStrings extracts target and replacement strings from tool arguments,
// supporting common LLM aliases (e.g. old_content/new_content, search/replace).
func ExtractEditStrings(args map[string]any) (string, string, bool) {
	var target, replacement string
	targetKeys := []string{"target_content", "old_content", "old_str", "search", "target"}
	for _, k := range targetKeys {
		if s, ok := args[k].(string); ok && s != "" {
			target = s
			break
		}
	}
	replacementKeys := []string{"replacement_content", "new_content", "new_str", "replace", "replacement"}
	for _, k := range replacementKeys {
		if s, ok := args[k].(string); ok {
			replacement = s
			break
		}
	}
	if target != "" && replacement != "" {
		return target, replacement, true
	}
	return "", "", false
}

// matchTrimmedLines finds a unique contiguous block of lines in fileLines matching targetLines ignoring leading/trailing whitespace.
func matchTrimmedLines(fileLines []string, targetLines []string) (int, int, bool) {
	if len(targetLines) == 0 || len(fileLines) < len(targetLines) {
		return -1, -1, false
	}
	trimmedTarget := make([]string, len(targetLines))
	for i, l := range targetLines {
		trimmedTarget[i] = strings.TrimSpace(l)
	}

	matchStart := -1
	matchEnd := -1
	matchCount := 0

	n := len(targetLines)
	for i := 0; i <= len(fileLines)-n; i++ {
		matched := true
		for j := 0; j < n; j++ {
			if strings.TrimSpace(fileLines[i+j]) != trimmedTarget[j] {
				matched = false
				break
			}
		}
		if matched {
			matchCount++
			matchStart = i
			matchEnd = i + n - 1
		}
	}

	if matchCount == 1 {
		return matchStart, matchEnd, true
	}
	return -1, -1, false
}

// ApplyFileEdits applies a list of replacement chunks to file content with resilient
// fallback matching (e.g. single-occurrence file-wide search) and clear small-file write_file guidance.
func ApplyFileEdits(content string, edits []ReplacementChunk, relPath string) (string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")

	for _, edit := range edits {
		edit.TargetContent = strings.ReplaceAll(edit.TargetContent, "\r\n", "\n")
		edit.ReplacementContent = strings.ReplaceAll(edit.ReplacementContent, "\r\n", "\n")

		start := edit.StartLine
		end := edit.EndLine
		if start < 1 {
			start = 1
		}
		if end <= 0 || end > len(lines) {
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

		// Resilient Fallback 3: Indentation-tolerant contiguous line block match
		targetLines := strings.Split(edit.TargetContent, "\n")
		if startIdx, endIdx, ok := matchTrimmedLines(lines, targetLines); ok {
			replLines := strings.Split(edit.ReplacementContent, "\n")
			newLines := append([]string{}, lines[:startIdx]...)
			newLines = append(newLines, replLines...)
			newLines = append(newLines, lines[endIdx+1:]...)
			lines = newLines
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
