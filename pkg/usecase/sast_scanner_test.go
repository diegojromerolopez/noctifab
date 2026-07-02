package usecase

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSAST_Disabled(t *testing.T) {
	s := &SASTScanner{
		Config: SASTConfig{Enabled: false},
	}
	result, err := s.Run(context.Background(), ".")
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Empty(t, result.Issues)
}

func TestSAST_NoScanners(t *testing.T) {
	s := &SASTScanner{
		Config: SASTConfig{
			Enabled:  true,
			Scanners: []string{},
		},
	}
	result, err := s.Run(context.Background(), ".")
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Empty(t, result.Issues)
}

func TestSAST_NonexistentScanner(t *testing.T) {
	s := &SASTScanner{
		Config: SASTConfig{
			Enabled:        true,
			Scanners:       []string{"nonexistent-scanner"},
			FailOnSeverity: "high",
		},
	}
	result, err := s.Run(context.Background(), ".")
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Empty(t, result.Issues)
}

func TestSAST_GosecNotInstalled(t *testing.T) {
	gosecErr := exec.Command("gosec").Run()
	if gosecErr == nil {
		t.Skip("gosec is installed on this system; test requires it to be absent")
	}
	s := &SASTScanner{
		Config: SASTConfig{
			Enabled:        true,
			Scanners:       []string{"gosec"},
			FailOnSeverity: "high",
		},
	}
	_, err := s.Run(context.Background(), ".")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SAST: gosec failed")
	assert.True(t, errors.Is(err, exec.ErrNotFound))
}

func TestSeverityScore_high(t *testing.T) {
	s := &SASTScanner{}
	assert.Equal(t, 3, s.severityScore("high"))
	assert.Equal(t, 3, s.severityScore("HIGH"))
	assert.Equal(t, 3, s.severityScore("High"))
}

func TestSeverityScore_medium(t *testing.T) {
	s := &SASTScanner{}
	assert.Equal(t, 2, s.severityScore("medium"))
	assert.Equal(t, 2, s.severityScore("MEDIUM"))
}

func TestSeverityScore_low(t *testing.T) {
	s := &SASTScanner{}
	assert.Equal(t, 1, s.severityScore("low"))
}

func TestSeverityScore_unknown(t *testing.T) {
	s := &SASTScanner{}
	assert.Equal(t, 0, s.severityScore("critical"))
	assert.Equal(t, 0, s.severityScore(""))
	assert.Equal(t, 0, s.severityScore("info"))
}

func TestIsBlocking_HighWhenHigh(t *testing.T) {
	s := &SASTScanner{
		Config: SASTConfig{FailOnSeverity: "high"},
	}
	assert.True(t, s.isBlockingSeverity("high"))
}

func TestIsBlocking_MediumWhenHigh(t *testing.T) {
	s := &SASTScanner{
		Config: SASTConfig{FailOnSeverity: "high"},
	}
	assert.False(t, s.isBlockingSeverity("medium"))
}

func TestIsBlocking_LowWhenHigh(t *testing.T) {
	s := &SASTScanner{
		Config: SASTConfig{FailOnSeverity: "high"},
	}
	assert.False(t, s.isBlockingSeverity("low"))
}

func TestIsBlocking_HighWhenMedium(t *testing.T) {
	s := &SASTScanner{
		Config: SASTConfig{FailOnSeverity: "medium"},
	}
	assert.True(t, s.isBlockingSeverity("high"))
}

func TestIsBlocking_MediumWhenMedium(t *testing.T) {
	s := &SASTScanner{
		Config: SASTConfig{FailOnSeverity: "medium"},
	}
	assert.True(t, s.isBlockingSeverity("medium"))
}

func TestIsBlocking_LowWhenMedium(t *testing.T) {
	s := &SASTScanner{
		Config: SASTConfig{FailOnSeverity: "medium"},
	}
	assert.False(t, s.isBlockingSeverity("low"))
}

func TestParseGosecJSON(t *testing.T) {
	input := `{"Issues":[{"severity":"HIGH","file":"/app/main.go","line":"42","details":"Potential SQL injection"}]}`
	issues, err := parseGosecJSON(input)
	require.NoError(t, err)
	require.Len(t, issues, 1)

	assert.Equal(t, "gosec", issues[0].Scanner)
	assert.Equal(t, "high", issues[0].Severity)
	assert.Equal(t, "/app/main.go", issues[0].File)
	assert.Equal(t, 42, issues[0].Line)
	assert.Equal(t, "Potential SQL injection", issues[0].Description)
}

func TestParseGosecJSON_Empty(t *testing.T) {
	issues, err := parseGosecJSON("")
	require.NoError(t, err)
	assert.Nil(t, issues)
}

func TestParseBanditJSON(t *testing.T) {
	input := `{"results":[{"issue_severity":"MEDIUM","filename":"/app/app.py","line_number":10,"issue_text":"Hardcoded password"}]}`
	issues, err := parseBanditJSON(input)
	require.NoError(t, err)
	require.Len(t, issues, 1)

	assert.Equal(t, "bandit", issues[0].Scanner)
	assert.Equal(t, "medium", issues[0].Severity)
	assert.Equal(t, "/app/app.py", issues[0].File)
	assert.Equal(t, 10, issues[0].Line)
	assert.Equal(t, "Hardcoded password", issues[0].Description)
}

func TestParseBanditJSON_Empty(t *testing.T) {
	issues, err := parseBanditJSON("")
	require.NoError(t, err)
	assert.Nil(t, issues)
}
