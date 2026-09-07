package services

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyFileEdits_StandardRange(t *testing.T) {
	orig := "line 1\nline 2\nline 3\nline 4"
	edits := []ReplacementChunk{
		{
			StartLine:          2,
			EndLine:            3,
			TargetContent:      "line 2",
			ReplacementContent: "modified 2",
		},
	}

	res, err := ApplyFileEdits(orig, edits, "app.py")
	require.NoError(t, err)
	assert.Contains(t, res, "modified 2")
	assert.Contains(t, res, "line 1")
}

func TestApplyFileEdits_ResilientUniqueMatchOutOfRange(t *testing.T) {
	orig := "line 1\nline 2\nline 3\nfrom frontpunch.exceptions import ConnectionError"
	// Agent gave start_line: 1, end_line: 2, but target is on line 4
	edits := []ReplacementChunk{
		{
			StartLine:          1,
			EndLine:            2,
			TargetContent:      "from frontpunch.exceptions import ConnectionError",
			ReplacementContent: "from frontpunch.exceptions import ConnectionError, SerializationError",
		},
	}

	res, err := ApplyFileEdits(orig, edits, "client.py")
	require.NoError(t, err)
	assert.Contains(t, res, "SerializationError")
}

func TestApplyFileEdits_SmallFileFailureRecommendsWriteFile(t *testing.T) {
	orig := "line 1\nline 2\nline 3"
	edits := []ReplacementChunk{
		{
			StartLine:          1,
			EndLine:            3,
			TargetContent:      "nonexistent content",
			ReplacementContent: "replacement",
		},
	}

	_, err := ApplyFileEdits(orig, edits, "small.py")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "call 'write_file' with path='small.py'")
	assert.Contains(t, err.Error(), "<= 300 lines")
}

func TestApplyFileEdits_LargeFileFailure(t *testing.T) {
	var lines []string
	for i := 0; i < 350; i++ {
		lines = append(lines, "content line")
	}
	orig := strings.Join(lines, "\n")
	edits := []ReplacementChunk{
		{
			StartLine:          1,
			EndLine:            5,
			TargetContent:      "missing",
			ReplacementContent: "replacement",
		},
	}

	_, err := ApplyFileEdits(orig, edits, "large.py")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Call read_file first to get current content")
}
