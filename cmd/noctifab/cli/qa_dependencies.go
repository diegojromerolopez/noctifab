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
	var buildSandbox services.QABuildSandbox
	var sandboxRunner services.QASandboxRunner

	if cfg.Sandbox.Mode == "host" {
		buildSandbox = services.NewHostQABuildSandbox(process, fsys, timeout)
		sandboxRunner = services.NewHostQASandboxRunner(process, fsys, timeout)
	} else {
		buildSandbox = services.NewDockerQABuildSandbox(process, fsys, "noctifab-sandbox", timeout, 64, "512m")
		sandboxRunner = services.NewDockerQASandboxRunner(process, fsys, "noctifab-sandbox", 64, "512m")
	}

	return []services.QADependencies{{
		WorkspaceFactory: services.NewGitReviewWorkspaceFactory(git, fsys, clock),
		ArtifactBuilder:  services.NewArtifactBuildRunner(buildSandbox, fsys),
		Sandbox:          sandboxRunner,
		FileSystem:       fsys,
		Clock:            clock,
	}}
}
