package reporting

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectStoryContracts_Subdirectory(t *testing.T) {
	projDir := t.TempDir()
	storiesDir := filepath.Join(projDir, "roadmap", "user-stories")
	require.NoError(t, os.MkdirAll(storiesDir, 0755))

	contractContent := "```noctifab-contract\n{\n  \"public_contracts\": [\n    {\"id\": \"API-001\", \"interface\": \"http\"}\n  ]\n}\n```"
	storyPath := filepath.Join(storiesDir, "US-001-api.md")
	require.NoError(t, os.WriteFile(storyPath, []byte(contractContent), 0644))

	contracts := collectStoryContracts(projDir)
	require.Len(t, contracts, 1)
	assert.Equal(t, "API-001", contracts[0].ID)
	assert.Equal(t, "http", contracts[0].Interface)
}
