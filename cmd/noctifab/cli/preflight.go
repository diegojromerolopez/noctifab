package cli

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
)

const maxAllowedPingLatency = 10 * time.Second

// getRequiredSandboxBinaries extracts all unique binary executables needed by the sandbox configuration.
func getRequiredSandboxBinaries(cfg *config.Config) []string {
	seen := make(map[string]bool)
	var list []string
	add := func(bin string) {
		bin = strings.TrimSpace(bin)
		if bin == "" {
			return
		}
		fields := strings.Fields(bin)
		if len(fields) == 0 {
			return
		}
		exe := fields[0]
		// If executable is a relative or absolute file path (e.g. ./bin/app, bin/test, /usr/local/bin/cmd),
		// do not treat it as a host $PATH binary requirement.
		if strings.HasPrefix(exe, ".") || strings.HasPrefix(exe, "/") || strings.Contains(exe, "/") || strings.Contains(exe, "\\") {
			return
		}
		binName := filepath.Base(exe)
		if !seen[binName] {
			seen[binName] = true
			list = append(list, binName)
		}
	}

	add(cfg.Sandbox.TestCommand)
	if cfg.Sandbox.Linter.Command != nil {
		add(*cfg.Sandbox.Linter.Command)
	} else if cfg.Sandbox.LinterCommand != nil {
		add(*cfg.Sandbox.LinterCommand)
	}
	add(cfg.Sandbox.FormatterCommand)

	return list
}

// runPreFlightChecks executes environment and LLM provider connectivity diagnostics
// before orchestrator launch, banning unreachable or slow providers, failing on missing host binaries,
// and verifying release & quality gate non-triviality and language consistency.
func runPreFlightChecks(cfg *config.Config, projectDir ...string) error {
	fmt.Println("Running pre-flight checks...")
	pDir := "."
	if len(projectDir) > 0 && projectDir[0] != "" {
		pDir = projectDir[0]
	}

	tools := []string{"go", "docker", "python3", "rustc", "make", "gcc"}
	var foundTools []string
	for _, t := range tools {
		if _, err := exec.LookPath(t); err == nil {
			foundTools = append(foundTools, t)
		}
	}
	fmt.Printf("- Sandbox build tools available: %s\n", strings.Join(foundTools, ", "))

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
			fmt.Printf("- Configured sandbox binaries available: %s\n", strings.Join(available, ", "))
		}
		if len(missing) > 0 {
			if cfg.Sandbox.Mode == "" || strings.EqualFold(cfg.Sandbox.Mode, "host") {
				return fmt.Errorf("pre-flight check failed: required sandbox binary not found on host $PATH: %s", strings.Join(missing, ", "))
			}
			fmt.Printf("⚠️  Warning: missing configured sandbox binaries on host (mode=%s): %s\n", cfg.Sandbox.Mode, strings.Join(missing, ", "))
		}
	}

	requiredToolchains := DetectRequiredProjectToolchains(pDir)
	if len(requiredToolchains) > 0 {
		var missingToolchains []string
		var availableToolchains []string
		for _, bin := range requiredToolchains {
			if _, err := exec.LookPath(bin); err == nil {
				availableToolchains = append(availableToolchains, bin)
			} else {
				missingToolchains = append(missingToolchains, bin)
			}
		}
		if len(availableToolchains) > 0 {
			fmt.Printf("- Project required toolchains available: %s\n", strings.Join(availableToolchains, ", "))
		}
		if len(missingToolchains) > 0 {
			if cfg.Sandbox.Mode == "" || strings.EqualFold(cfg.Sandbox.Mode, "host") {
				return fmt.Errorf("pre-flight check failed: required toolchain binary not found on host $PATH: %s (please install the required toolchain or configure sandbox.mode: docker)", strings.Join(missingToolchains, ", "))
			}
			fmt.Printf("⚠️  Warning: missing required toolchain binaries on host (mode=%s): %s\n", cfg.Sandbox.Mode, strings.Join(missingToolchains, ", "))
		}
	}

	if len(cfg.LLM.Providers) > 0 {
		var activeProviders []config.ProviderSpec
		var bannedNames []string

		for _, p := range cfg.LLM.Providers {
			fmt.Printf("- LLM provider (%s / %s) ping: ", p.Name, p.Provider)
			latency, err := llm.Ping(context.Background(), p.Provider, p.APIKeyValue, p.URL)
			if err != nil {
				low := strings.ToLower(err.Error())
				if strings.Contains(low, "401") || strings.Contains(low, "402") || strings.Contains(low, "credit") || strings.Contains(low, "unauthorized") {
					fmt.Printf("BANNED ⚠️ (CREDIT EXHAUSTED / AUTH ERROR: %v)\n", err)
				} else {
					fmt.Printf("BANNED (unreachable: %v)\n", err)
				}
				bannedNames = append(bannedNames, p.Name)
				continue
			}
			if latency > maxAllowedPingLatency {
				fmt.Printf("BANNED (latency %dms exceeds 10s threshold)\n", latency.Milliseconds())
				bannedNames = append(bannedNames, p.Name)
				continue
			}
			fmt.Printf("OK (%dms)\n", latency.Milliseconds())
			activeProviders = append(activeProviders, p)
		}

		if len(activeProviders) == 0 {
			return fmt.Errorf("pre-flight check failed: all configured LLM providers were banned or unreachable")
		}

		cfg.LLM.Providers = activeProviders
		if len(cfg.LLM.Priority) > 0 {
			var filteredPriority []string
			for _, name := range cfg.LLM.Priority {
				banned := false
				for _, b := range bannedNames {
					if strings.EqualFold(name, b) {
						banned = true
						break
					}
				}
				if !banned {
					filteredPriority = append(filteredPriority, name)
				}
			}
			cfg.LLM.Priority = filteredPriority
		}
	} else if len(cfg.LLMs) > 0 {
		var activeLLMs []config.LLMConfig
		for _, p := range cfg.LLMs {
			fmt.Printf("- LLM provider (%s) ping: ", p.Provider)
			latency, err := llm.Ping(context.Background(), p.Provider, p.APIKeyValue, p.URL)
			if err != nil {
				fmt.Printf("BANNED (unreachable: %v)\n", err)
				continue
			}
			if latency > maxAllowedPingLatency {
				fmt.Printf("BANNED (latency %dms exceeds 10s threshold)\n", latency.Milliseconds())
				continue
			}
			fmt.Printf("OK (%dms)\n", latency.Milliseconds())
			activeLLMs = append(activeLLMs, p)
		}
		if len(activeLLMs) == 0 {
			return fmt.Errorf("pre-flight check failed: all configured LLM providers were banned or unreachable")
		}
		cfg.LLMs = activeLLMs
	} else {
		fmt.Printf("- LLM provider (%s) ping: ", cfg.LLM.Provider)
		latency, err := llm.Ping(context.Background(), cfg.LLM.Provider, cfg.LLM.APIKeyValue, cfg.LLM.URL)
		if err != nil {
			fmt.Printf("FAIL: %v\n", err)
			return fmt.Errorf("pre-flight LLM provider ping failed: %w", err)
		}
		if latency > maxAllowedPingLatency {
			fmt.Printf("BANNED (latency %dms exceeds 10s threshold)\n", latency.Milliseconds())
			return fmt.Errorf("pre-flight LLM provider %s banned: latency %dms exceeds 10s threshold", cfg.LLM.Provider, latency.Milliseconds())
		}
		fmt.Printf("OK (%dms)\n", latency.Milliseconds())
	}
	fmt.Printf("- Sandbox mode (%s): OK\n", cfg.Sandbox.Mode)
	if err := VerifyQualityAndReleaseGates(cfg, pDir); err != nil {
		return err
	}
	fmt.Println("Pre-flight checks passed successfully.")
	return nil
}
