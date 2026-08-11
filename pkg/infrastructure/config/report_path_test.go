package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

func TestResolveReportPath(t *testing.T) {
	fixedTime := time.Date(2026, 8, 11, 22, 51, 22, 0, time.UTC)
	project := "/work/project"

	t.Run("when key is omitted or whitespace", func(t *testing.T) {
		t.Run("it disables reporting without error", func(t *testing.T) {
			path, enabled, err := config.ResolveReportPathWithTime(project, "", fixedTime)
			require.NoError(t, err)
			assert.False(t, enabled)
			assert.Empty(t, path)

			path, enabled, err = config.ResolveReportPathWithTime(project, "   \t\n", fixedTime)
			require.NoError(t, err)
			assert.False(t, enabled)
			assert.Empty(t, path)
		})
	})

	t.Run("when relative path inside .noctifab/reports is supplied", func(t *testing.T) {
		t.Run("it resolves path and formats filename with date and project folder", func(t *testing.T) {
			path, enabled, err := config.ResolveReportPathWithTime(project, ".noctifab/reports/report.md", fixedTime)
			require.NoError(t, err)
			assert.True(t, enabled)
			assert.Equal(t, "/work/project/.noctifab/reports/20260811_225122_project.md", path)
		})
	})

	t.Run("when relative path is outside .noctifab/reports", func(t *testing.T) {
		t.Run("it rejects README.md inside workspace root", func(t *testing.T) {
			_, enabled, err := config.ResolveReportPathWithTime(project, "README.md", fixedTime)
			assert.Error(t, err)
			assert.False(t, enabled)
		})

		t.Run("it rejects .git/report.md", func(t *testing.T) {
			_, enabled, err := config.ResolveReportPathWithTime(project, ".git/report.md", fixedTime)
			assert.Error(t, err)
			assert.False(t, enabled)
		})
	})

	t.Run("when external absolute path is supplied", func(t *testing.T) {
		t.Run("it resolves path and prefixes custom filename basename", func(t *testing.T) {
			path, enabled, err := config.ResolveReportPathWithTime(project, "/tmp/noctifab-run.md", fixedTime)
			require.NoError(t, err)
			assert.True(t, enabled)
			assert.Equal(t, "/tmp/20260811_225122_project_noctifab-run.md", path)
		})
	})

	t.Run("when path contains NUL byte", func(t *testing.T) {
		t.Run("it returns error", func(t *testing.T) {
			_, enabled, err := config.ResolveReportPathWithTime(project, ".noctifab/reports/\x00report.md", fixedTime)
			assert.Error(t, err)
			assert.False(t, enabled)
		})
	})
}
