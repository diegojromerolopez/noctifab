package domain

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAction_UnmarshalJSON_ToolAliases(t *testing.T) {
	t.Run("standard tool field", func(t *testing.T) {
		js := `{"tool":"write_file","args":{"path":"src/lib.rs"}}`
		var act Action
		err := json.Unmarshal([]byte(js), &act)
		require.NoError(t, err)
		assert.Equal(t, "write_file", act.Tool)
	})

	t.Run("cmd field alias", func(t *testing.T) {
		js := `{"cmd":"write_file","args":{"path":"Cargo.toml"}}`
		var act Action
		err := json.Unmarshal([]byte(js), &act)
		require.NoError(t, err)
		assert.Equal(t, "write_file", act.Tool)
	})

	t.Run("name field alias", func(t *testing.T) {
		js := `{"name":"list_directory","args":{"path":"src"}}`
		var act Action
		err := json.Unmarshal([]byte(js), &act)
		require.NoError(t, err)
		assert.Equal(t, "list_directory", act.Tool)
	})

	t.Run("command field alias", func(t *testing.T) {
		js := `{"command":"read_file","args":{"path":"SPEC.md"}}`
		var act Action
		err := json.Unmarshal([]byte(js), &act)
		require.NoError(t, err)
		assert.Equal(t, "read_file", act.Tool)
	})
}
