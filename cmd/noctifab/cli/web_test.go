package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWebCmd_Flags(t *testing.T) {
	assert.Equal(t, "web [workspace_dir]", webCmd.Use)
	assert.NotNil(t, webCmd.Flags().Lookup("host"))
	assert.NotNil(t, webCmd.Flags().Lookup("port"))
	assert.NotNil(t, webCmd.Flags().Lookup("readonly"))
}
