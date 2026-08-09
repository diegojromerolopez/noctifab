package services_test

import (
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/require"
)

func qaContract() domain.StoryContract {
	return domain.StoryContract{PublicContracts: []domain.PublicContract{{
		ID:                 "cli.invalid-input",
		AllowedExecutables: []string{"./dist/example"},
		ExitCodes:          []int{2},
		StdoutContains:     []string{"usage"},
		StderrPrefixes:     []string{"ERROR:"},
	}}}
}

func qaResponse(scenarios []any) *domain.LLMResponse {
	return &domain.LLMResponse{Actions: []domain.LLMAction{{
		Tool: "propose_scenarios",
		Args: map[string]any{"scenarios": scenarios},
	}}}
}

func qaScenario(name string) map[string]any {
	return map[string]any{
		"name":               name,
		"public_contract_id": "cli.invalid-input",
		"steps": []any{map[string]any{
			"command":            []string{"./dist/example", "--input", ""},
			"stdin":              "",
			"expected_exit_code": 2,
			"stdout_contains":    []string{"usage"},
			"stderr_prefix":      "ERROR:",
		}},
	}
}

func TestParseQAScenarioProposal(t *testing.T) {
	t.Run("when the response contains one valid action it normalizes and fingerprints scenarios", func(t *testing.T) {
		scenarios, duplicates, err := services.ParseQAScenarioProposal(qaResponse([]any{qaScenario(" empty-input ")}), qaContract(), 8)
		require.NoError(t, err)
		require.Zero(t, duplicates)
		require.Len(t, scenarios, 1)
		require.Equal(t, "empty-input", scenarios[0].Name)
		require.Len(t, scenarios[0].Fingerprint, 64)
	})

	t.Run("when scenarios differ only by name it retains the first and counts the duplicate", func(t *testing.T) {
		scenarios, duplicates, err := services.ParseQAScenarioProposal(qaResponse([]any{qaScenario("first"), qaScenario("second")}), qaContract(), 8)
		require.NoError(t, err)
		require.Equal(t, 1, duplicates)
		require.Len(t, scenarios, 1)
		require.Equal(t, "first", scenarios[0].Name)
	})

	tests := []struct {
		name     string
		response *domain.LLMResponse
		limit    int
		needle   string
	}{
		{"when the response is nil", nil, 8, "exactly one action"},
		{"when another tool is requested", &domain.LLMResponse{Actions: []domain.LLMAction{{Tool: "run_tests"}}}, 8, "propose_scenarios"},
		{"when scenarios are empty", qaResponse(nil), 8, "must not be empty"},
		{"when the bound is exceeded", qaResponse([]any{qaScenario("one"), qaScenario("two")}), 1, "exceeds limit"},
		{"when action args contain unknown fields", &domain.LLMResponse{Actions: []domain.LLMAction{{Tool: "propose_scenarios", Args: map[string]any{"scenarios": []any{qaScenario("one")}, "status": "PASS"}}}}, 8, "unknown field"},
	}
	for _, test := range tests {
		t.Run(test.name+" it rejects the proposal", func(t *testing.T) {
			_, _, err := services.ParseQAScenarioProposal(test.response, qaContract(), test.limit)
			require.ErrorContains(t, err, test.needle)
		})
	}

	t.Run("when a scenario invents an expectation it is rejected", func(t *testing.T) {
		input := qaScenario("invented")
		input["steps"].([]any)[0].(map[string]any)["expected_exit_code"] = 99
		_, _, err := services.ParseQAScenarioProposal(qaResponse([]any{input}), qaContract(), 8)
		require.ErrorContains(t, err, "not declared")
	})

	t.Run("when a scenario chooses another executable it is rejected", func(t *testing.T) {
		input := qaScenario("forbidden")
		input["steps"].([]any)[0].(map[string]any)["command"] = []string{"sh", "-c", "true"}
		_, _, err := services.ParseQAScenarioProposal(qaResponse([]any{input}), qaContract(), 8)
		require.ErrorContains(t, err, "not allowed")
	})
}
