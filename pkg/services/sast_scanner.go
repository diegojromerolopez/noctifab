package services

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/telemetry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type SASTConfig struct {
	Enabled        bool     `yaml:"enabled"`
	Scanners       []string `yaml:"scanners"`
	FailOnSeverity string   `yaml:"fail_on_severity"`
}

type SecurityIssue struct {
	Scanner     string `json:"scanner"`
	Severity    string `json:"severity"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Description string `json:"description"`
}

type SASTResult struct {
	Passed bool            `json:"passed"`
	Issues []SecurityIssue `json:"issues"`
}

type SASTScanner struct {
	Config SASTConfig
}

func (s *SASTScanner) Run(ctx context.Context, projectPath string) (*SASTResult, error) {
	ctx, span := telemetry.Tracer().Start(ctx, "Run",
		trace.WithAttributes(
			attribute.Bool("enabled", s.Config.Enabled),
			attribute.StringSlice("scanners", s.Config.Scanners),
			attribute.String("fail_on_severity", s.Config.FailOnSeverity),
		))
	defer span.End()

	if !s.Config.Enabled {
		return &SASTResult{Passed: true}, nil
	}

	var allIssues []SecurityIssue

	for _, scanner := range s.Config.Scanners {
		switch scanner {
		case "gosec":
			issues, err := s.runGosec(ctx, projectPath)
			if err != nil {
				return nil, fmt.Errorf("SAST: gosec failed: %w", err)
			}
			allIssues = append(allIssues, issues...)
		case "bandit":
			issues, err := s.runBandit(ctx, projectPath)
			if err != nil {
				return nil, fmt.Errorf("SAST: bandit failed: %w", err)
			}
			allIssues = append(allIssues, issues...)
		}
	}

	blocked := false
	for _, issue := range allIssues {
		if s.isBlockingSeverity(issue.Severity) {
			blocked = true
			break
		}
	}

	return &SASTResult{
		Passed: !blocked,
		Issues: allIssues,
	}, nil
}

func (s *SASTScanner) severityScore(sev string) int {
	switch strings.ToLower(sev) {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func (s *SASTScanner) isBlockingSeverity(sev string) bool {
	return s.severityScore(sev) >= s.severityScore(s.Config.FailOnSeverity)
}

func (s *SASTScanner) runGosec(ctx context.Context, projectPath string) ([]SecurityIssue, error) {
	cmd := exec.CommandContext(ctx, "gosec", "-fmt", "json", "./...")
	cmd.Dir = projectPath
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, err
		}
	}
	return parseGosecJSON(string(output))
}

func (s *SASTScanner) runBandit(ctx context.Context, projectPath string) ([]SecurityIssue, error) {
	cmd := exec.CommandContext(ctx, "bandit", "-r", "-f", "json", ".")
	cmd.Dir = projectPath
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return nil, err
		}
	}
	return parseBanditJSON(string(output))
}

func parseGosecJSON(output string) ([]SecurityIssue, error) {
	if output == "" {
		return nil, nil
	}
	var issues []SecurityIssue
	scanner := "gosec"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, `"severity"`) {
			sev := extractJSONValue(line, "severity")
			f := extractJSONValue(line, "file")
			lineStr := extractJSONValue(line, "line")
			desc := extractJSONValue(line, "details")
			lineNum, _ := strconv.Atoi(lineStr)
			issues = append(issues, SecurityIssue{
				Scanner:     scanner,
				Severity:    strings.ToLower(sev),
				File:        f,
				Line:        lineNum,
				Description: desc,
			})
		}
	}
	if len(issues) == 0 {
		return nil, nil
	}
	return issues, nil
}

func parseBanditJSON(output string) ([]SecurityIssue, error) {
	if output == "" {
		return nil, nil
	}
	var issues []SecurityIssue
	scanner := "bandit"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, `"issue_severity"`) {
			sev := extractJSONValue(line, "issue_severity")
			f := extractJSONValue(line, "filename")
			lineStr := extractJSONValue(line, "line_number")
			desc := extractJSONValue(line, "issue_text")
			lineNum, _ := strconv.Atoi(lineStr)
			issues = append(issues, SecurityIssue{
				Scanner:     scanner,
				Severity:    strings.ToLower(sev),
				File:        f,
				Line:        lineNum,
				Description: desc,
			})
		}
	}
	if len(issues) == 0 {
		return nil, nil
	}
	return issues, nil
}

func extractJSONValue(line, key string) string {
	search := fmt.Sprintf(`"%s"`, key)
	idx := strings.Index(line, search)
	if idx < 0 {
		return ""
	}
	after := line[idx+len(search):]
	after = strings.TrimPrefix(after, ": ")
	after = strings.TrimPrefix(after, ":")
	after = strings.TrimSpace(after)

	if strings.HasPrefix(after, `"`) {
		after = after[1:]
		end := strings.Index(after, `"`)
		if end < 0 {
			return after
		}
		return after[:end]
	}
	end := strings.Index(after, ",")
	if end < 0 {
		end = strings.Index(after, "}")
	}
	if end < 0 {
		return after
	}
	return strings.TrimSpace(after[:end])
}
