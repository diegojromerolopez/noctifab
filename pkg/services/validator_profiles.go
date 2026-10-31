package services

import (
	"fmt"
	"os"
	"regexp"
)

var defaultRoleProfiles = map[string]ProfileConfig{
	"orchestrator": {
		AllowedTools:    []string{"*"},
		AllowedCommands: []string{"*"},
	},
	"planner": {
		AllowedTools:    []string{"add_task", "log_message", "noop"},
		AllowedCommands: []string{},
	},
	"tester": {
		AllowedTools:    []string{"read_file", "write_file", "write_files", "edit_file", "apply_patch", "delete_file", "list_directory", "find_files", "grep_search", "run_tests", "run_e2e_tests", "run_linter", "noop"},
		AllowedCommands: []string{},
	},
	"generator": {
		AllowedTools:    []string{"read_file", "write_file", "write_files", "edit_file", "apply_patch", "delete_file", "list_directory", "find_files", "grep_search", "run_tests", "run_e2e_tests", "run_linter", "request_test_fix", "noop"},
		AllowedCommands: []string{},
	},
	"qa": {
		AllowedTools:    []string{"read_file", "write_file", "write_files", "edit_file", "list_directory", "find_files", "grep_search", "run_tests", "run_e2e_tests", "noop"},
		AllowedCommands: []string{"*"},
	},
}

func compileForbiddenPatterns(patterns []string) []*regexp.Regexp {
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		if p == "" {
			continue
		}
		re, err := regexp.Compile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: invalid forbidden_pattern regex %q: %v (skipping)\n", p, err)
			continue
		}
		compiled = append(compiled, re)
	}
	return compiled
}

// findForbiddenPattern returns the first matching forbidden pattern's source string.
func (v *PolicyValidator) findForbiddenPattern(content string) string {
	for _, re := range v.forbiddenPatterns {
		if re.MatchString(content) {
			return re.String()
		}
	}
	return ""
}
