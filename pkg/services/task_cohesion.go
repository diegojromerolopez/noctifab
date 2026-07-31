package services

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// ValidateTaskCohesion enforces that no DAG task is defined purely for interfaces
// or abstractions without including its corresponding concrete implementation file.
func ValidateTaskCohesion(tasks []domain.Task) error {
	for _, task := range tasks {
		if isInterfaceOnlyTask(task) {
			return fmt.Errorf("task cohesion violation in task %q (%s): interface definitions must be paired with implementation files in target_files", task.ID, task.Title)
		}
	}
	return nil
}

func isInterfaceOnlyTask(task domain.Task) bool {
	if len(task.TargetFiles) == 0 {
		return false
	}

	interfaceOnly := true
	hasInterfaceFile := false

	for _, file := range task.TargetFiles {
		base := strings.ToLower(filepath.Base(file))
		if isInterfaceFilename(base) {
			hasInterfaceFile = true
		} else {
			interfaceOnly = false
		}
	}

	if hasInterfaceFile && interfaceOnly {
		// If task title or description explicitly indicates interface-only without implementation
		titleLower := strings.ToLower(task.Title)
		if strings.Contains(titleLower, "interface") || strings.Contains(titleLower, "stub") || strings.Contains(titleLower, "definition") {
			return true
		}
		return true
	}

	return false
}

func isInterfaceFilename(base string) bool {
	return strings.HasSuffix(base, "_interface.go") ||
		strings.HasSuffix(base, "_interfaces.go") ||
		base == "interface.go" ||
		base == "interfaces.go" ||
		strings.HasSuffix(base, "_stub.go")
}
