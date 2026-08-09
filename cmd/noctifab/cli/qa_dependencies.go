package cli

import (
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/services"
)

func qaDependencies(cfg *config.Config) []services.QADependencies {
	if cfg == nil || !cfg.Agents.QA.Enabled {
		return nil
	}
	process := services.ExecQAProcessRunner{}
	fsys := services.OSQAFileSystem{}
	clock := services.SystemQAClock{}
	timeout := time.Duration(cfg.Agents.QA.MaxDuration)
	git := services.NewExecReviewGitRunner(process, timeout)
	buildSandbox := services.NewDockerQABuildSandbox(process, fsys, "noctifab-sandbox", timeout, 64, "512m")
	return []services.QADependencies{{
		WorkspaceFactory: services.NewGitReviewWorkspaceFactory(git, fsys, clock),
		ArtifactBuilder:  services.NewArtifactBuildRunner(buildSandbox, fsys),
		Sandbox:          services.NewDockerQASandboxRunner(process, fsys, "noctifab-sandbox", 64, "512m"),
		FileSystem:       fsys,
		Clock:            clock,
	}}
}
