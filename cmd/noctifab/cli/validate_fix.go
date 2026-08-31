package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/llm"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	fixFlag = false
	yesFlag = false
)

var yamlFenceRE = regexp.MustCompile(`(?s)` + "```" + `(?:ya?ml)?\s*(.*?)\s*` + "```")

func init() {
	validateCmd.Flags().BoolVarP(&fixFlag, "fix", "f", false, "Automatically diagnose and repair configuration errors using AI")
	validateCmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Automatically accept and apply AI-repaired configuration without prompting")
}

// runAIConfigFix coordinates the AI configuration diagnosis and repair loop.
func runAIConfigFix(cmd *cobra.Command, configPath string, parseErr error, autoYes bool) error {
	fmt.Printf("\n❌ Configuration validation error detected in %s:\n   %v\n\n", configPath, parseErr)
	fmt.Println("🔧 Invoking AI Configuration Repair Assistant...")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read configuration file %s: %w", configPath, err)
	}

	repairClient, providerName, err := resolveRepairClient(configPath)
	if err != nil {
		return fmt.Errorf("cannot run AI repair: %w", err)
	}

	fmt.Printf("✔ Connected to LLM provider [%s] for configuration repair.\n", providerName)
	fmt.Println("⏳ Generating corrected configuration...")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	repairedYAML, err := RepairConfigWithAI(ctx, string(data), parseErr, repairClient)
	if err != nil {
		return fmt.Errorf("AI configuration repair failed: %w", err)
	}

	// Verify repaired YAML locally
	if _, err := config.ValidateBytes([]byte(repairedYAML)); err != nil {
		return fmt.Errorf("repaired configuration failed local schema validation: %w", err)
	}

	fmt.Println("✔ Repaired configuration parsed and validated cleanly!")
	fmt.Println("\n--- Proposed Changes ---")
	printSimpleDiff(string(data), repairedYAML)
	fmt.Println("------------------------")

	if !autoYes {
		fmt.Printf("\nApply these changes to %s? (A backup will be saved to %s.bak) [Y/n]: ", configPath, configPath)
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "" && input != "y" && input != "yes" {
			fmt.Println("❌ Aborted without applying changes.")
			return nil
		}
	}

	// Create backup
	bakPath := configPath + ".bak"
	if err := os.WriteFile(bakPath, data, 0644); err != nil {
		return fmt.Errorf("failed to create backup file %s: %w", bakPath, err)
	}
	fmt.Printf("✔ Backup saved to %s\n", bakPath)

	// Write repaired file
	if err := os.WriteFile(configPath, []byte(repairedYAML), 0644); err != nil {
		return fmt.Errorf("failed to write repaired configuration to %s: %w", configPath, err)
	}
	fmt.Printf("✔ Successfully repaired %s!\n\n", configPath)

	return nil
}

// RepairConfigWithAI sends the broken configuration and parser error to an LLM for correction.
func RepairConfigWithAI(ctx context.Context, brokenYAML string, parseErr error, client domain.LLMClient) (string, error) {
	if client == nil {
		return "", fmt.Errorf("no LLM client provided for repair")
	}

	prompt := fmt.Sprintf(`You are an expert Noctifab Configuration AI Repair Engine.
The user's Noctifab YAML configuration file failed validation.

PARSER / VALIDATION ERROR:
%v

BROKEN YAML CONTENT:
`+"```yaml"+`
%s
`+"```"+`

CANONICAL NOCTIFAB CONFIGURATION RULES & SCHEMA:
1. Root version: config_version: '2.0'
2. Valid top-level sections: config_version, agents, runtime, storage, llm, vcs, sandbox, workspace_cache, telemetry, context, logging, poll_interval, max_clarification_wait, clarification_timeout_action, execution_report.
3. Agent roles: orchestrator, product_manager, planner, generators, testers, qa, auditor, unblocker, last_resort.
4. Ensemble topologies (under agents.<role>.ensemble or roles.<role>.ensemble):
   - strategy: 'parallel' | 'serial' | 'consensus' | 'race' | 'cascade' | 'decomposed' | 'best_of_n_scored'
   - models: list of model objects (e.g. - name: <provider>, optional model, temperature, max_tokens, enable_thinking, extra_params)
   - min_models: integer (speculative quorum threshold)
   - soft_timeout_seconds: integer
   - early_exit_on_pass: boolean (for serial refinement)
   - stages: list of stages for serial (with name, refinement_prompt)
   - voters: list of voters for consensus (with name)
   - tie_breaker: object with name for consensus
   - tiers: list of tiers for cascade
   - targets: list of targets for decomposed (with name, role_prompt)
5. Token ceilings: max_tokens: -1 (unlimited), max_tokens_per_story: -1.
6. Fix all indentation errors, typos, unrecognized keys, and schema mismatches.
7. Preserve all existing provider configurations, API keys, secrets, model names, comments, and valid settings.
8. Output ONLY the repaired YAML document enclosed in a single `+"```yaml"+` code block.
`, parseErr, brokenYAML)

	resp, err := client.Complete(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("LLM completion error: %w", err)
	}

	content := ""
	if resp != nil {
		if len(resp.Actions) > 0 {
			for _, act := range resp.Actions {
				if c, ok := act.Args["content"].(string); ok && strings.TrimSpace(c) != "" {
					content = c
					break
				}
			}
		}
		if content == "" {
			content = resp.Reasoning
		}
	}

	matches := yamlFenceRE.FindStringSubmatch(content)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1]) + "\n", nil
	}

	if strings.Contains(content, "config_version:") {
		return strings.TrimSpace(content) + "\n", nil
	}

	return "", fmt.Errorf("LLM did not return a valid YAML code block")
}

func resolveRepairClient(configPath string) (domain.LLMClient, string, error) {
	// Merge secrets from global and local secrets.yaml
	secrets := make(map[string]string)
	if homeSecretsPath := config.GlobalSecretsPath(); homeSecretsPath != "" {
		if s, err := config.LoadSecrets(homeSecretsPath); err == nil {
			for k, v := range s {
				secrets[k] = v
			}
		}
	}
	if s, err := config.LoadSecrets(".noctifab/secrets.yaml"); err == nil {
		for k, v := range s {
			secrets[k] = v
		}
	}

	resolveKey := func(keyNames ...string) string {
		for _, kn := range keyNames {
			if kn == "" {
				continue
			}
			if val, ok := secrets[kn]; ok && strings.TrimSpace(val) != "" {
				return val
			}
			if val := os.Getenv(kn); strings.TrimSpace(val) != "" {
				return val
			}
		}
		return ""
	}

	// 1. Try to pick the FIRST LLM provider defined directly in the configuration file
	if data, err := os.ReadFile(configPath); err == nil {
		var raw struct {
			LLM struct {
				Provider  string                 `yaml:"provider"`
				Model     string                 `yaml:"model"`
				URL       string                 `yaml:"url"`
				Providers []config.ProviderSpec  `yaml:"providers"`
			} `yaml:"llm"`
		}
		_ = yaml.Unmarshal(data, &raw)

		// Check the first provider in llm.providers
		if len(raw.LLM.Providers) > 0 {
			first := raw.LLM.Providers[0]
			key := first.APIKeyValue
			if key == "" {
				var keyCandidates []string
				keyCandidates = append(keyCandidates, first.APIKeys...)
				keyCandidates = append(keyCandidates, strings.ToUpper(first.Provider)+"_API_KEY")
				keyCandidates = append(keyCandidates, strings.ToUpper(first.Name)+"_API_KEY")
				key = resolveKey(keyCandidates...)
			}
			if key != "" || strings.HasPrefix(first.URL, "http://localhost") || strings.HasPrefix(first.URL, "http://127.0.0.1") {
				c := llm.NewClient(first.Provider, first.Model, key, 2, 50*time.Millisecond, first.URL)
				return c, fmt.Sprintf("%s (%s / %s)", first.Name, first.Provider, first.Model), nil
			}
		}

		// Check top-level llm.provider
		if raw.LLM.Provider != "" {
			key := resolveKey(strings.ToUpper(raw.LLM.Provider) + "_API_KEY")
			if key != "" || strings.HasPrefix(raw.LLM.URL, "http://localhost") {
				c := llm.NewClient(raw.LLM.Provider, raw.LLM.Model, key, 2, 50*time.Millisecond, raw.LLM.URL)
				return c, fmt.Sprintf("%s (%s)", raw.LLM.Provider, raw.LLM.Model), nil
			}
		}
	}

	// 2. Fallback: check environment or secrets for the first available provider
	keyChecks := []struct {
		envVar   string
		provider string
		model    string
	}{
		{"CLAUDE_API_KEY", "anthropic", "claude-3-5-sonnet-20241022"},
		{"ANTHROPIC_API_KEY", "anthropic", "claude-3-5-sonnet-20241022"},
		{"OPENAI_API_KEY", "openai", "gpt-4o"},
		{"GEMINI_API_KEY", "gemini", "gemini-2.5-pro"},
		{"DEEPSEEK_API_KEY", "deepseek", "deepseek-chat"},
		{"QWENCLOUD_API_KEY", "qwencloud", "qwen3.8-max"},
		{"OPENCODE_API_KEY", "opencode", "glm-5.2"},
		{"OPENROUTER_API_KEY", "openrouter", "deepseek/deepseek-v4-flash-0731"},
	}

	for _, kc := range keyChecks {
		if val := resolveKey(kc.envVar); val != "" {
			c := llm.NewClient(kc.provider, kc.model, val, 2, 50*time.Millisecond, "")
			return c, kc.provider, nil
		}
	}

	return nil, "", fmt.Errorf("no valid LLM API keys found in environment or secrets.yaml to perform AI repair")
}

func printSimpleDiff(original, repaired string) {
	origLines := strings.Split(original, "\n")
	repLines := strings.Split(repaired, "\n")

	max := len(origLines)
	if len(repLines) > max {
		max = len(repLines)
	}

	for i := 0; i < max; i++ {
		orig := ""
		if i < len(origLines) {
			orig = origLines[i]
		}
		rep := ""
		if i < len(repLines) {
			rep = repLines[i]
		}
		if orig != rep {
			if orig != "" {
				fmt.Printf(" \033[31m- %s\033[0m\n", orig)
			}
			if rep != "" {
				fmt.Printf(" \033[32m+ %s\033[0m\n", rep)
			}
		}
	}
}
