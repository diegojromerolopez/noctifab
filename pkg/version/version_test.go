package version

import (
	"encoding/json"
	"runtime/debug"
	"strings"
	"testing"
)

func TestVersionInfo(t *testing.T) {
	// Backup original state
	origVersion := Version
	origCommit := GitCommit
	origDate := CommitDate
	origReader := BuildInfoReader

	defer func() {
		Version = origVersion
		GitCommit = origCommit
		CommitDate = origDate
		BuildInfoReader = origReader
	}()

	t.Run("when ldflags are explicitly set, it uses those values", func(t *testing.T) {
		Version = "1.2.3"
		GitCommit = "abcdef1234567890"
		CommitDate = "2026-08-16T20:00:00Z"
		BuildInfoReader = func() (*debug.BuildInfo, bool) {
			return nil, false
		}

		info := GetInfo()
		if info.Version != "1.2.3" {
			t.Errorf("expected version 1.2.3, got %s", info.Version)
		}
		if info.GitCommit != "abcdef1234567890" {
			t.Errorf("expected commit abcdef1234567890, got %s", info.GitCommit)
		}
		if info.CommitDate != "2026-08-16T20:00:00Z" {
			t.Errorf("expected date 2026-08-16T20:00:00Z, got %s", info.CommitDate)
		}
		if info.Dirty {
			t.Errorf("expected dirty=false, got true")
		}

		// Test Short
		if info.Short() != "1.2.3" {
			t.Errorf("expected Short()=1.2.3, got %s", info.Short())
		}

		// Test String formatting with short commit hash (first 7 chars)
		str := info.String()
		expectedStr := "noctifab version 1.2.3 (commit: abcdef1, date: 2026-08-16T20:00:00Z)"
		if str != expectedStr {
			t.Errorf("expected String()=%q, got %q", expectedStr, str)
		}

		// Test Verbose formatting
		verbose := info.Verbose()
		if !strings.Contains(verbose, "Version:     1.2.3") ||
			!strings.Contains(verbose, "Git Commit:  abcdef1234567890") ||
			!strings.Contains(verbose, "Commit Date: 2026-08-16T20:00:00Z") {
			t.Errorf("unexpected Verbose() output:\n%s", verbose)
		}

		// Test JSON formatting
		jsonStr, err := info.JSON()
		if err != nil {
			t.Fatalf("JSON() returned error: %v", err)
		}
		var parsed Info
		if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
			t.Fatalf("failed to unmarshal JSON output: %v", err)
		}
		if parsed.Version != "1.2.3" || parsed.GitCommit != "abcdef1234567890" || parsed.CommitDate != "2026-08-16T20:00:00Z" {
			t.Errorf("parsed JSON mismatch: %+v", parsed)
		}
	})

	t.Run("when build info is available and ldflags are empty, it extracts from build info", func(t *testing.T) {
		Version = ""
		GitCommit = ""
		CommitDate = ""
		BuildInfoReader = func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{
				Main: debug.Module{
					Version: "v0.99.0",
				},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "1234567890abcdef"},
					{Key: "vcs.time", Value: "2026-08-15T12:00:00Z"},
					{Key: "vcs.modified", Value: "true"},
				},
			}, true
		}

		info := GetInfo()
		if info.Version != "v0.99.0" {
			t.Errorf("expected version v0.99.0, got %s", info.Version)
		}
		if info.GitCommit != "1234567890abcdef" {
			t.Errorf("expected commit 1234567890abcdef, got %s", info.GitCommit)
		}
		if info.CommitDate != "2026-08-15T12:00:00Z" {
			t.Errorf("expected date 2026-08-15T12:00:00Z, got %s", info.CommitDate)
		}
		if !info.Dirty {
			t.Errorf("expected dirty=true, got false")
		}

		// String should include -dirty suffix
		str := info.String()
		expectedStr := "noctifab version v0.99.0 (commit: 1234567-dirty, date: 2026-08-15T12:00:00Z)"
		if str != expectedStr {
			t.Errorf("expected String()=%q, got %q", expectedStr, str)
		}

		// Verbose should include (dirty)
		verbose := info.Verbose()
		if !strings.Contains(verbose, "Git Commit:  1234567890abcdef (dirty)") {
			t.Errorf("expected verbose output to contain dirty mark, got:\n%s", verbose)
		}
	})

	t.Run("when neither ldflags nor build info are provided, it uses defaults", func(t *testing.T) {
		Version = ""
		GitCommit = ""
		CommitDate = ""
		BuildInfoReader = func() (*debug.BuildInfo, bool) {
			return nil, false
		}

		info := GetInfo()
		if info.Version != DefaultVersion {
			t.Errorf("expected version %s, got %s", DefaultVersion, info.Version)
		}
		if info.GitCommit != "" {
			t.Errorf("expected empty commit, got %s", info.GitCommit)
		}
		if info.CommitDate != "" {
			t.Errorf("expected empty date, got %s", info.CommitDate)
		}

		str := info.String()
		expectedStr := "noctifab version " + DefaultVersion
		if str != expectedStr {
			t.Errorf("expected String()=%q, got %q", expectedStr, str)
		}
	})

	t.Run("when build info main version is (devel), it falls back to DefaultVersion", func(t *testing.T) {
		Version = ""
		GitCommit = ""
		CommitDate = ""
		BuildInfoReader = func() (*debug.BuildInfo, bool) {
			return &debug.BuildInfo{
				Main: debug.Module{
					Version: "(devel)",
				},
			}, true
		}

		info := GetInfo()
		if info.Version != DefaultVersion {
			t.Errorf("expected version %s, got %s", DefaultVersion, info.Version)
		}
	})

	t.Run("when short commit is exactly 7 characters, it handles slicing correctly", func(t *testing.T) {
		Version = "1.0.0"
		GitCommit = "1234567"
		CommitDate = ""
		BuildInfoReader = func() (*debug.BuildInfo, bool) {
			return nil, false
		}

		info := GetInfo()
		str := info.String()
		expectedStr := "noctifab version 1.0.0 (commit: 1234567)"
		if str != expectedStr {
			t.Errorf("expected String()=%q, got %q", expectedStr, str)
		}
	})

	t.Run("when only commit date is set without commit, it formats properly", func(t *testing.T) {
		Version = "1.0.0"
		GitCommit = ""
		CommitDate = "2026-08-16"
		BuildInfoReader = func() (*debug.BuildInfo, bool) {
			return nil, false
		}

		info := GetInfo()
		str := info.String()
		expectedStr := "noctifab version 1.0.0 (date: 2026-08-16)"
		if str != expectedStr {
			t.Errorf("expected String()=%q, got %q", expectedStr, str)
		}
	})
}
