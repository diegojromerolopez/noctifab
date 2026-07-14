package services

import (
	"context"
	"errors"
	"os"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// DeleteFileTool implements delete_file.
type DeleteFileTool struct{}

func (t *DeleteFileTool) Name() string { return "delete_file" }
func (t *DeleteFileTool) Description() string {
	return "delete_file deletes a file in the workspace. Arguments: path (string)."
}
func (t *DeleteFileTool) Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error) {
	path, ok := args["path"].(string)
	if !ok || path == "" {
		return "", errors.New("missing or invalid 'path' argument")
	}
	fullPath, err := resolveSandboxPath(state.ProjectPath, path)
	if err != nil {
		return "", err
	}
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return "File does not exist", nil
		}
		return "", err
	}
	return "File deleted successfully", nil
}
