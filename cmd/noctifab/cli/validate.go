package cli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:           "validate",
	Short:         "Validate configuration, state, and directory constraints",
	SilenceErrors: true,
	SilenceUsage:  true,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load(cmd)
		if err != nil {
			return err
		}

		fmt.Println("Validating configuration...")
		fmt.Println("Configuration is valid.")
		fmt.Println("✔ Configuration loaded successfully.")
		for _, role := range config.RoleCapabilities(cfg) {
			fmt.Printf("role %s: %s\n", role.Name, role.Capability)
		}

		tools := []string{"go", "docker", "python3", "rustc", "make", "gcc"}
		var foundTools []string
		for _, t := range tools {
			if _, err := exec.LookPath(t); err == nil {
				foundTools = append(foundTools, t)
			}
		}
		fmt.Printf("✔ Sandbox build tools available: %s\n", strings.Join(foundTools, ", "))

		requiredBinaries := getRequiredSandboxBinaries(cfg)
		if len(requiredBinaries) > 0 {
			var missing []string
			var available []string
			for _, bin := range requiredBinaries {
				if _, err := exec.LookPath(bin); err == nil {
					available = append(available, bin)
				} else {
					missing = append(missing, bin)
				}
			}
			if len(available) > 0 {
				fmt.Printf("✔ Configured sandbox binaries available: %s\n", strings.Join(available, ", "))
			}
			if len(missing) > 0 {
				fmt.Printf("⚠️  Warning: missing configured sandbox binaries on host (mode=%s): %s\n", cfg.Sandbox.Mode, strings.Join(missing, ", "))
			}
		}

		if len(cfg.LLM.Providers) > 0 {
			for _, p := range cfg.LLM.Providers {
				fmt.Printf("- Pinging provider %s (%s)... ", p.Name, p.Provider)
				latency, err := llm.Ping(context.Background(), p.Provider, p.APIKeyValue, p.URL)
				if err != nil {
					low := strings.ToLower(err.Error())
					if strings.Contains(low, "401") || strings.Contains(low, "402") || strings.Contains(low, "credit") || strings.Contains(low, "unauthorized") {
						fmt.Printf("⚠️ CREDIT EXHAUSTED / AUTH ERROR: %v\n", err)
					} else {
						fmt.Printf("❌ ERROR: %v\n", err)
					}
				} else {
					fmt.Printf("✔ OK (%dms)\n", latency.Milliseconds())
				}
			}
		}

		return nil
	},
}

func init() {
	RootCmd.AddCommand(validateCmd)
}
