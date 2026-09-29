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

func TestExtractEditStrings_Aliases(t *testing.T) {
	// Standard
	t1, r1, ok1 := ExtractEditStrings(map[string]any{
		"target_content":      "old code",
		"replacement_content": "new code",
	})
	assert.True(t, ok1)
	assert.Equal(t, "old code", t1)
	assert.Equal(t, "new code", r1)

	// old_content / new_content
	t2, r2, ok2 := ExtractEditStrings(map[string]any{
		"old_content": "old code",
		"new_content": "new code",
	})
	assert.True(t, ok2)
	assert.Equal(t, "old code", t2)
	assert.Equal(t, "new code", r2)

	// search / replace
	t3, r3, ok3 := ExtractEditStrings(map[string]any{
		"search":  "old code",
		"replace": "new code",
	})
	assert.True(t, ok3)
	assert.Equal(t, "old code", t3)
	assert.Equal(t, "new code", r3)

	// Missing
	_, _, ok4 := ExtractEditStrings(map[string]any{"other": "val"})
	assert.False(t, ok4)
}

func TestApplyFileEdits_IndentationTolerant(t *testing.T) {
	orig := "def dispatch(cmd):\n    if cmd == 'PING':\n        return b'+PONG\\r\\n'\n    return b'-ERR\\r\\n'"
	// Target content has different leading spaces and extra whitespace
	target := "  if cmd == 'PING':\n      return b'+PONG\\r\\n'"
	repl := "    if cmd == 'PING':\n        return b'+PONG\\r\\n'\n    if cmd == 'ECHO':\n        return b'+OK\\r\\n'"

	res, err := ApplyFileEdits(orig, []ReplacementChunk{{TargetContent: target, ReplacementContent: repl}}, "cmd.py")
	require.NoError(t, err)
	assert.Contains(t, res, "ECHO")
	assert.Contains(t, res, "PONG")
}

func TestApplyFileEdits_CRLFNormalization(t *testing.T) {
	orig := "line 1\r\nline 2\r\nline 3\r\n"
	target := "line 2\n"
	repl := "modified 2\n"

	res, err := ApplyFileEdits(orig, []ReplacementChunk{{TargetContent: target, ReplacementContent: repl}}, "crlf.py")
	require.NoError(t, err)
	assert.Contains(t, res, "modified 2")
}
