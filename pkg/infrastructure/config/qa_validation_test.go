package config

import (
	"strings"
	"testing"
	"time"
)

func TestQAConfigValidation(t *testing.T) {
	valid := func() *Config {
		cfg := DefaultConfig()
		cfg.VCS.TokenValue = "vcs-token"
		cfg.LLM.APIKeyValue = "llm-key"
		cfg.Agents.QA.Enabled = true
		cfg.Agents.QA.ValidationCommands = []string{"./dist/example"}
		return cfg
	}

	tests := []struct {
		name string
		edit func(*Config)
		want string
	}{
		{name: "valid documented config"},
		{name: "valid boundary values", edit: func(cfg *Config) {
			cfg.Agents.QA.Iterations = 3
			cfg.Agents.QA.MaxReviewRounds = 5
			cfg.Agents.QA.MaxOutputBytes = 1048576
			cfg.Agents.QA.MaxCostUSD = "00012.3400"
			cfg.Agents.QA.ValidationCommands = []string{"bin/app", "./dist/tool"}
			cfg.Agents.QA.TesterPathPrefixes = []string{"test", "./specs/"}
		}},
		{name: "wrong architecture", edit: func(cfg *Config) { cfg.Agents.Architecture = "single_pass" }, want: "architecture"},
		{name: "worktrees disabled", edit: func(cfg *Config) { cfg.VCS.UseWorktrees = false }, want: "use_worktrees"},
		{name: "nonblocking", edit: func(cfg *Config) { cfg.Agents.QA.Blocking = false }, want: "blocking"},
		{name: "network enabled", edit: func(cfg *Config) { cfg.Agents.QA.Network = "bridge" }, want: "network"},
		{name: "zero iterations", edit: func(cfg *Config) { cfg.Agents.QA.Iterations = 0 }, want: "iterations"},
		{name: "too many iterations", edit: func(cfg *Config) { cfg.Agents.QA.Iterations = 4 }, want: "iterations"},
		{name: "zero duration", edit: func(cfg *Config) { cfg.Agents.QA.MaxDuration = 0 }, want: "max_duration"},
		{name: "negative duration", edit: func(cfg *Config) { cfg.Agents.QA.MaxDuration = Duration(-time.Second) }, want: "max_duration"},
		{name: "zero scenarios", edit: func(cfg *Config) { cfg.Agents.QA.MaxScenarios = 0 }, want: "max_scenarios"},
		{name: "zero review rounds", edit: func(cfg *Config) { cfg.Agents.QA.MaxReviewRounds = 0 }, want: "max_review_rounds"},
		{name: "too many review rounds", edit: func(cfg *Config) { cfg.Agents.QA.MaxReviewRounds = 6 }, want: "max_review_rounds"},
		{name: "output below minimum", edit: func(cfg *Config) { cfg.Agents.QA.MaxOutputBytes = 1023 }, want: "max_output_bytes"},
		{name: "output above maximum", edit: func(cfg *Config) { cfg.Agents.QA.MaxOutputBytes = 1048577 }, want: "max_output_bytes"},
		{name: "empty budget", edit: func(cfg *Config) { cfg.Agents.QA.MaxCostUSD = "" }, want: "max_cost_usd"},
		{name: "negative budget", edit: func(cfg *Config) { cfg.Agents.QA.MaxCostUSD = "-1" }, want: "max_cost_usd"},
		{name: "signed budget", edit: func(cfg *Config) { cfg.Agents.QA.MaxCostUSD = "+1" }, want: "max_cost_usd"},
		{name: "exponent budget", edit: func(cfg *Config) { cfg.Agents.QA.MaxCostUSD = "1e3" }, want: "max_cost_usd"},
		{name: "multiple decimal points", edit: func(cfg *Config) { cfg.Agents.QA.MaxCostUSD = "1.2.3" }, want: "max_cost_usd"},
		{name: "missing decimal digits", edit: func(cfg *Config) { cfg.Agents.QA.MaxCostUSD = "1." }, want: "max_cost_usd"},
		{name: "empty build command", edit: func(cfg *Config) { cfg.Agents.QA.BuildCommand = nil }, want: "build_command"},
		{name: "blank build executable", edit: func(cfg *Config) { cfg.Agents.QA.BuildCommand = []string{"  "} }, want: "build_command"},
		{name: "empty validation list", edit: func(cfg *Config) { cfg.Agents.QA.ValidationCommands = nil }, want: "validation_commands"},
		{name: "empty validation executable", edit: func(cfg *Config) { cfg.Agents.QA.ValidationCommands = []string{""} }, want: "validation_commands"},
		{name: "absolute validation executable", edit: func(cfg *Config) { cfg.Agents.QA.ValidationCommands = []string{"/bin/app"} }, want: "validation_commands"},
		{name: "validation arguments", edit: func(cfg *Config) { cfg.Agents.QA.ValidationCommands = []string{"bin/app --check"} }, want: "validation_commands"},
		{name: "validation traversal", edit: func(cfg *Config) { cfg.Agents.QA.ValidationCommands = []string{"bin/../app"} }, want: "validation_commands"},
		{name: "unclean validation path", edit: func(cfg *Config) { cfg.Agents.QA.ValidationCommands = []string{"bin//app"} }, want: "validation_commands"},
		{name: "backslash validation path", edit: func(cfg *Config) { cfg.Agents.QA.ValidationCommands = []string{`bin\app`} }, want: "validation_commands"},
		{name: "empty tester prefix list", edit: func(cfg *Config) { cfg.Agents.QA.TesterPathPrefixes = nil }, want: "tester_path_prefixes"},
		{name: "empty tester prefix", edit: func(cfg *Config) { cfg.Agents.QA.TesterPathPrefixes = []string{""} }, want: "tester_path_prefixes"},
		{name: "absolute tester prefix", edit: func(cfg *Config) { cfg.Agents.QA.TesterPathPrefixes = []string{"/tests/"} }, want: "tester_path_prefixes"},
		{name: "tester prefix traversal", edit: func(cfg *Config) { cfg.Agents.QA.TesterPathPrefixes = []string{"tests/../spec/"} }, want: "tester_path_prefixes"},
		{name: "unclean tester prefix", edit: func(cfg *Config) { cfg.Agents.QA.TesterPathPrefixes = []string{"tests//unit/"} }, want: "tester_path_prefixes"},
		{name: "backslash tester prefix", edit: func(cfg *Config) { cfg.Agents.QA.TesterPathPrefixes = []string{`tests\unit`} }, want: "tester_path_prefixes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid()
			if tt.edit != nil {
				tt.edit(cfg)
			}
			err := cfg.Validate()
			if tt.want == "" && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if tt.want != "" && (err == nil || !strings.Contains(err.Error(), tt.want)) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestDisabledQAConfigDoesNotApplyEnabledConstraints(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VCS.TokenValue = "vcs-token"
	cfg.LLM.APIKeyValue = "llm-key"
	cfg.Agents.Architecture = "single_pass"
	cfg.VCS.UseWorktrees = false
	cfg.Agents.QA = QAConfig{Enabled: false}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled QA config must remain loadable: %v", err)
	}
}
