package cli

import (
	"context"
	"strings"
	"testing"
)

func TestResolveToolchainStrategy(t *testing.T) {
	ctx := context.Background()

	t.Run("explicit docker strategy", func(t *testing.T) {
		got := ResolveToolchainStrategy(ctx, "docker", nil)
		if got != ToolchainStrategyDocker {
			t.Errorf("expected docker, got %s", got)
		}
	})

	t.Run("explicit local strategy", func(t *testing.T) {
		got := ResolveToolchainStrategy(ctx, "local", nil)
		if got != ToolchainStrategyLocal {
			t.Errorf("expected local, got %s", got)
		}
	})

	t.Run("explicit off strategy", func(t *testing.T) {
		for _, s := range []string{"off", "none", "disabled"} {
			got := ResolveToolchainStrategy(ctx, s, nil)
			if got != ToolchainStrategyOff {
				t.Errorf("for %s, expected off, got %s", s, got)
			}
		}
	})

	t.Run("auto strategy with docker available", func(t *testing.T) {
		mockDocker := func(ctx context.Context) bool { return true }
		got := ResolveToolchainStrategy(ctx, "auto", mockDocker)
		if got != ToolchainStrategyDocker {
			t.Errorf("expected docker when available, got %s", got)
		}
	})

	t.Run("auto strategy with docker unavailable", func(t *testing.T) {
		mockDocker := func(ctx context.Context) bool { return false }
		got := ResolveToolchainStrategy(ctx, "auto", mockDocker)
		if got != ToolchainStrategyLocal {
			t.Errorf("expected local when docker unavailable, got %s", got)
		}
	})

	t.Run("empty string defaults to auto", func(t *testing.T) {
		mockDocker := func(ctx context.Context) bool { return true }
		got := ResolveToolchainStrategy(ctx, "", mockDocker)
		if got != ToolchainStrategyDocker {
			t.Errorf("expected docker when default auto and docker available, got %s", got)
		}
	})
}

func TestDetectMissingToolchainIndicator(t *testing.T) {
	tests := []struct {
		log      string
		expected bool
	}{
		{"exec: \"cargo\": executable file not found in $PATH", true},
		{"make: cargo: command not found", true},
		{"sh: pytest: not found in $PATH", true},
		{"bash: /bin/rustc: No such file or directory", true},
		{"exit status 127", true},
		{"command failed with exit code 127", true},
		{"AssertionError: expected 1 to equal 2", false},
		{"TypeError: cannot read property of undefined", false},
	}

	for _, tc := range tests {
		got := DetectMissingToolchainIndicator(tc.log)
		if got != tc.expected {
			t.Errorf("for log %q, expected %v, got %v", tc.log, tc.expected, got)
		}
	}
}

func TestBuildToolchainFallbackDirective(t *testing.T) {
	t.Run("docker directive contains Dockerfile and makefile instructions", func(t *testing.T) {
		d := BuildToolchainFallbackDirective(ToolchainStrategyDocker, true)
		if !strings.Contains(d, "Dockerfile") {
			t.Errorf("expected Dockerfile mention in docker directive")
		}
		if !strings.Contains(d, "docker run --rm") {
			t.Errorf("expected docker run --rm in docker directive")
		}
		if !strings.Contains(d, "ATTENTION: Diagnostics indicate") {
			t.Errorf("expected detected missing banner")
		}
	})

	t.Run("local directive contains install_package instruction", func(t *testing.T) {
		d := BuildToolchainFallbackDirective(ToolchainStrategyLocal, false)
		if !strings.Contains(d, "install_package") {
			t.Errorf("expected install_package in local directive")
		}
	})

	t.Run("off directive is empty", func(t *testing.T) {
		d := BuildToolchainFallbackDirective(ToolchainStrategyOff, true)
		if d != "" {
			t.Errorf("expected empty directive for off strategy, got %q", d)
		}
	})
}
