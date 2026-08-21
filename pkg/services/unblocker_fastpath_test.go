package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFastPathClassify(t *testing.T) {
	t.Run("matches interactive stdin prompts", func(t *testing.T) {
		snippet := "npm WARN\n? Do you want to proceed with installing dependencies? (Y/n)"
		res := FastPathClassify(snippet)
		assert.True(t, res.Matched)
		assert.Equal(t, "interactive_stdin_prompt_wait", res.Reason)
		assert.Contains(t, res.Directive, "non-interactively")
	})

	t.Run("matches port binding collisions", func(t *testing.T) {
		snippet := "Starting server...\nError: listen tcp 127.0.0.1:8080: bind: address already in use"
		res := FastPathClassify(snippet)
		assert.True(t, res.Matched)
		assert.Equal(t, "port_binding_collision", res.Reason)
		assert.Contains(t, res.Directive, "port binding collision")
	})

	t.Run("matches interactive test watch mode", func(t *testing.T) {
		snippet := "PASS tests/unit/calculator.test.js\nWatch Usage: Press f to run failed tests"
		res := FastPathClassify(snippet)
		assert.True(t, res.Matched)
		assert.Equal(t, "interactive_watch_mode", res.Reason)
		assert.Contains(t, res.Directive, "--watchAll=false")
	})

	t.Run("matches missing toolchain binary", func(t *testing.T) {
		snippet := "sh: 1: pytest: not found\nexit status 127"
		res := FastPathClassify(snippet)
		assert.True(t, res.Matched)
		assert.Equal(t, "missing_toolchain_binary", res.Reason)
		assert.Contains(t, res.Directive, "standard library")
	})

	t.Run("returns unmatched for unrecognized log snippets", func(t *testing.T) {
		snippet := "Building package... Done in 2.1s"
		res := FastPathClassify(snippet)
		assert.False(t, res.Matched)
	})
}
