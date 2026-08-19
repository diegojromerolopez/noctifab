package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	specOutputFile      string
	specNonInteractive  bool
	specEnableConsensus bool
)

var specCmd = &cobra.Command{
	Use:   "spec [path_or_prompt...]",
	Short: "Create or interactively refine a project specification (SPEC.md)",
	Long: `Create or refine a project specification (SPEC.md) through an interactive
Human-in-the-Loop (HITL) review cycle.

If SPEC.md does not exist, it drafts a new specification from your prompt.
If SPEC.md exists, it refines the specification with your feedback.
If run without a prompt, it audits the existing SPEC.md and enters the review loop.

The engine uses a 4-stage multi-model drafting pipeline (Product Manager, Systems Architect,
Test Architect, QA Specialist) followed by a cross-model consensus audit to eliminate
single-provider bias and ensure technical consistency.

You can provide iterative feedback as many times as you like. The loop will stop when you
enter an approval phrase (e.g. "looks good to me", "all right, it's enough", "stop", "done").`,
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		targetDir, prompt := parseSpecArgs(args)
		return runSpecSession(targetDir, prompt, specOutputFile, specNonInteractive, specEnableConsensus)
	},
}

func init() {
	specCmd.Flags().StringVarP(&specOutputFile, "output", "o", "SPEC.md", "Target specification file output path")
	specCmd.Flags().BoolVar(&specNonInteractive, "non-interactive", false, "Run single pass and exit without interactive loop")
	specCmd.Flags().BoolVar(&specEnableConsensus, "consensus", true, "Enable multi-model consensus audit pass")

	RootCmd.AddCommand(specCmd)
}

func parseSpecArgs(args []string) (string, string) {
	if len(args) == 0 {
		return ".", ""
	}

	// Check if the first argument is an existing directory
	if info, err := os.Stat(args[0]); err == nil && info.IsDir() {
		targetDir := args[0]
		prompt := strings.Join(args[1:], " ")
		return targetDir, prompt
	}

	// Otherwise, entire argument list is the prompt and targetDir is "."
	return ".", strings.Join(args, " ")
}

func runSpecSession(targetDir, prompt, outputFile string, nonInteractive, consensus bool) error {
	var err error
	if targetDir == "" || targetDir == "." {
		targetDir, err = os.Getwd()
		if err != nil {
			return err
		}
	} else if !filepath.IsAbs(targetDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		targetDir = filepath.Join(cwd, targetDir)
	}

	targetFilePath := outputFile
	if !filepath.IsAbs(targetFilePath) {
		targetFilePath = filepath.Join(targetDir, targetFilePath)
	}

	cfg, err := loadWorkspaceConfigOrDefault(targetDir)
	if err != nil {
		cfg = config.DefaultConfig()
	}

	router := llm.NewResilientLLMRouter(cfg, nil)
	orchestrator := services.NewSpecOrchestrator(cfg, router, nil, nil)

	ctx := context.Background()
	_, err = orchestrator.RunSession(ctx, services.RunSessionOptions{
		ProjectPath:    targetDir,
		InitialPrompt:  prompt,
		TargetFile:     targetFilePath,
		NonInteractive: nonInteractive,
		EnableAudit:    consensus,
	})
	return err
}

func loadWorkspaceConfigOrDefault(dir string) (*config.Config, error) {
	cfg := config.DefaultConfig()
	cfgPath := filepath.Join(dir, ".noctifab", "config.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil && len(bytes.TrimSpace(data)) > 0 {
		_ = yaml.Unmarshal(data, cfg)
	}
	return cfg, nil
}
