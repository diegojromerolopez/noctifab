package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/version"
)

func TestVersionCmd(t *testing.T) {
	// Set test version variables
	origVersion := version.Version
	origCommit := version.GitCommit
	origDate := version.CommitDate

	version.Version = "0.35.0"
	version.GitCommit = "abcdef1234567890"
	version.CommitDate = "2026-08-16T22:00:00Z"
	RootCmd.Version = version.GetInfo().String()

	defer func() {
		version.Version = origVersion
		version.GitCommit = origCommit
		version.CommitDate = origDate
		versionCmd.SetOut(nil)
		versionCmd.SetErr(nil)
		RootCmd.SetOut(nil)
		RootCmd.SetErr(nil)
	}()

	t.Run("when version command is run without flags, it outputs single-line summary", func(t *testing.T) {
		buf := new(bytes.Buffer)
		versionCmd.SetOut(buf)
		versionCmd.SetErr(buf)

		// Reset flags
		versionJSON = false
		versionShort = false
		versionVerbose = false

		err := versionCmd.RunE(versionCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		expected := "noctifab version 0.35.0 (commit: abcdef1, date: 2026-08-16T22:00:00Z)\n"
		if out != expected {
			t.Errorf("expected output %q, got %q", expected, out)
		}
	})

	t.Run("when version command is run with --short, it outputs only version number", func(t *testing.T) {
		buf := new(bytes.Buffer)
		versionCmd.SetOut(buf)
		versionCmd.SetErr(buf)

		versionJSON = false
		versionShort = true
		versionVerbose = false

		err := versionCmd.RunE(versionCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		expected := "0.35.0\n"
		if out != expected {
			t.Errorf("expected output %q, got %q", expected, out)
		}
	})

	t.Run("when version command is run with --verbose, it outputs detailed key-value metadata", func(t *testing.T) {
		buf := new(bytes.Buffer)
		versionCmd.SetOut(buf)
		versionCmd.SetErr(buf)

		versionJSON = false
		versionShort = false
		versionVerbose = true

		err := versionCmd.RunE(versionCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "Version:     0.35.0") ||
			!strings.Contains(out, "Git Commit:  abcdef1234567890") ||
			!strings.Contains(out, "Commit Date: 2026-08-16T22:00:00Z") ||
			!strings.Contains(out, "Go Version:") {
			t.Errorf("unexpected verbose output:\n%s", out)
		}
	})

	t.Run("when version command is run with --json, it outputs valid JSON", func(t *testing.T) {
		buf := new(bytes.Buffer)
		versionCmd.SetOut(buf)
		versionCmd.SetErr(buf)

		versionJSON = true
		versionShort = false
		versionVerbose = false

		err := versionCmd.RunE(versionCmd, []string{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		var parsed version.Info
		if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
			t.Fatalf("failed to parse JSON output %q: %v", buf.String(), err)
		}

		if parsed.Version != "0.35.0" || parsed.GitCommit != "abcdef1234567890" || parsed.CommitDate != "2026-08-16T22:00:00Z" {
			t.Errorf("parsed JSON mismatch: %+v", parsed)
		}
	})

	t.Run("when executing via RootCmd with version args, it executes successfully", func(t *testing.T) {
		buf := new(bytes.Buffer)
		versionJSON = false
		versionShort = true
		versionVerbose = false
		versionCmd.SetOut(nil)
		versionCmd.SetErr(nil)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"version", "--short"})

		err := RootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "0.35.0") {
			t.Errorf("expected 0.35.0 in output, got %q", out)
		}
	})

	t.Run("when executing via RootCmd with --version flag, it executes successfully", func(t *testing.T) {
		buf := new(bytes.Buffer)
		versionJSON = false
		versionShort = false
		versionVerbose = false
		versionCmd.SetOut(nil)
		versionCmd.SetErr(nil)
		RootCmd.SetOut(buf)
		RootCmd.SetErr(buf)
		RootCmd.SetArgs([]string{"--version"})

		err := RootCmd.Execute()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, "0.35.0") || !strings.Contains(out, "abcdef1") {
			t.Errorf("expected version and commit in output, got %q", out)
		}
	})
}
