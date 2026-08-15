package cli

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
)

const maxAllowedPingLatency = 10 * time.Second

// runPreFlightChecks executes environment and LLM provider connectivity diagnostics
// before orchestrator launch, banning unreachable or slow providers.
func runPreFlightChecks(cfg *config.Config) error {
	fmt.Println("Running pre-flight checks...")
	tools := []string{"go", "docker", "python3", "rustc", "make", "gcc"}
	var foundTools []string
	for _, t := range tools {
		if _, err := exec.LookPath(t); err == nil {
			foundTools = append(foundTools, t)
		}
	}
	fmt.Printf("- Sandbox build tools available: %s\n", strings.Join(foundTools, ", "))

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
	fmt.Println("Pre-flight checks passed successfully.")
	return nil
}
