package version

import (
	"encoding/json"
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

const (
	// DefaultVersion is the fallback version when not injected at build time.
	DefaultVersion = "0.38.0"
)

var (
	// Version is the application semantic release version.
	// Can be set at build time via: -ldflags "-X github.com/diegojromerolopez/noctifab/pkg/version.Version=..."
	Version = ""

	// GitCommit is the full Git commit hash.
	// Can be set at build time via: -ldflags "-X github.com/diegojromerolopez/noctifab/pkg/version.GitCommit=..."
	GitCommit = ""

	// CommitDate is the RFC3339 timestamp of the Git commit.
	// Can be set at build time via: -ldflags "-X github.com/diegojromerolopez/noctifab/pkg/version.CommitDate=..."
	CommitDate = ""

	// BuildInfoReader is an injection point for testing debug.ReadBuildInfo.
	BuildInfoReader = debug.ReadBuildInfo
)

// Info contains detailed build and version metadata.
type Info struct {
	Version    string `json:"version"`
	GitCommit  string `json:"git_commit,omitempty"`
	CommitDate string `json:"commit_date,omitempty"`
	GoVersion  string `json:"go_version"`
	Compiler   string `json:"compiler"`
	Platform   string `json:"platform"`
	Dirty      bool   `json:"dirty,omitempty"`
}

// GetInfo resolves the version, git commit, and commit date from build flags or Go runtime build info.
func GetInfo() Info {
	v := Version
	commit := GitCommit
	date := CommitDate
	dirty := false

	if bi, ok := BuildInfoReader(); ok {
		if v == "" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			v = bi.Main.Version
		}
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision":
				if commit == "" {
					commit = s.Value
				}
			case "vcs.time":
				if date == "" {
					date = s.Value
				}
			case "vcs.modified":
				if s.Value == "true" {
					dirty = true
				}
			}
		}
	}

	if v == "" {
		v = DefaultVersion
	}

	return Info{
		Version:    v,
		GitCommit:  commit,
		CommitDate: date,
		GoVersion:  runtime.Version(),
		Compiler:   runtime.Compiler,
		Platform:   fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Dirty:      dirty,
	}
}

// Short returns only the semantic version string.
func (i Info) Short() string {
	return i.Version
}

// String returns a single-line summary of the version, commit, and date.
func (i Info) String() string {
	var parts []string
	if i.GitCommit != "" {
		shortCommit := i.GitCommit
		if len(shortCommit) > 7 {
			shortCommit = shortCommit[:7]
		}
		if i.Dirty {
			shortCommit += "-dirty"
		}
		parts = append(parts, fmt.Sprintf("commit: %s", shortCommit))
	}
	if i.CommitDate != "" {
		parts = append(parts, fmt.Sprintf("date: %s", i.CommitDate))
	}

	if len(parts) > 0 {
		return fmt.Sprintf("noctifab version %s (%s)", i.Version, strings.Join(parts, ", "))
	}
	return fmt.Sprintf("noctifab version %s", i.Version)
}

// Verbose returns a structured multi-line string containing full metadata.
func (i Info) Verbose() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Version:     %s\n", i.Version)
	if i.GitCommit != "" {
		commitStr := i.GitCommit
		if i.Dirty {
			commitStr += " (dirty)"
		}
		fmt.Fprintf(&sb, "Git Commit:  %s\n", commitStr)
	}
	if i.CommitDate != "" {
		fmt.Fprintf(&sb, "Commit Date: %s\n", i.CommitDate)
	}
	fmt.Fprintf(&sb, "Go Version:  %s\n", i.GoVersion)
	fmt.Fprintf(&sb, "Compiler:    %s\n", i.Compiler)
	fmt.Fprintf(&sb, "Platform:    %s\n", i.Platform)
	return strings.TrimRight(sb.String(), "\n")
}

// JSON returns a formatted JSON representation of the version info.
func (i Info) JSON() (string, error) {
	bytes, err := json.MarshalIndent(i, "", "  ")
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}
