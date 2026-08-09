package services

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type qaProposal struct {
	Scenarios []qaScenarioInput `json:"scenarios"`
}

type qaScenarioInput struct {
	Name             string        `json:"name"`
	PublicContractID string        `json:"public_contract_id"`
	Steps            []qaStepInput `json:"steps"`
}

type qaStepInput struct {
	Command          []string `json:"command"`
	Stdin            string   `json:"stdin,omitempty"`
	ExpectedExitCode *int     `json:"expected_exit_code"`
	StdoutContains   []string `json:"stdout_contains,omitempty"`
	StderrPrefix     string   `json:"stderr_prefix,omitempty"`
}

// ParseQAScenarioProposal validates the QA response and suppresses duplicate fingerprints.
func ParseQAScenarioProposal(response *domain.LLMResponse, contract domain.StoryContract, maxScenarios int) ([]domain.QAScenario, int, error) {
	if response == nil || len(response.Actions) != 1 {
		return nil, 0, fmt.Errorf("qa scenarios: response must contain exactly one action")
	}
	action := response.Actions[0]
	if action.Tool != "propose_scenarios" {
		return nil, 0, fmt.Errorf("qa scenarios: action tool must be propose_scenarios")
	}

	data, err := json.Marshal(action.Args)
	if err != nil {
		return nil, 0, fmt.Errorf("qa scenarios: encode action args: %w", err)
	}
	var proposal qaProposal
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&proposal); err != nil {
		return nil, 0, fmt.Errorf("qa scenarios: invalid action args: %w", err)
	}
	if len(proposal.Scenarios) == 0 {
		return nil, 0, fmt.Errorf("qa scenarios: scenarios must not be empty")
	}
	if maxScenarios <= 0 || len(proposal.Scenarios) > maxScenarios {
		return nil, 0, fmt.Errorf("qa scenarios: scenario count %d exceeds limit %d", len(proposal.Scenarios), maxScenarios)
	}

	contracts := make(map[string]domain.PublicContract, len(contract.PublicContracts))
	for _, publicContract := range contract.PublicContracts {
		contracts[publicContract.ID] = publicContract
	}

	accepted := make([]domain.QAScenario, 0, len(proposal.Scenarios))
	seen := make(map[string]struct{}, len(proposal.Scenarios))
	duplicates := 0
	for i, input := range proposal.Scenarios {
		scenario, err := normalizeQAScenarioInput(input)
		if err != nil {
			return nil, 0, fmt.Errorf("qa scenarios: scenario %d: %w", i, err)
		}
		publicContract, exists := contracts[scenario.PublicContractID]
		if !exists {
			return nil, 0, fmt.Errorf("qa scenarios: scenario %q references unknown public contract %q", scenario.Name, scenario.PublicContractID)
		}
		if err := validateScenarioExpectations(scenario, publicContract); err != nil {
			return nil, 0, fmt.Errorf("qa scenarios: scenario %q: %w", scenario.Name, err)
		}
		fingerprint, err := QAScenarioFingerprint(scenario)
		if err != nil {
			return nil, 0, fmt.Errorf("qa scenarios: scenario %q fingerprint: %w", scenario.Name, err)
		}
		scenario.Fingerprint = fingerprint
		if _, duplicate := seen[fingerprint]; duplicate {
			duplicates++
			continue
		}
		seen[fingerprint] = struct{}{}
		accepted = append(accepted, scenario)
	}
	return accepted, duplicates, nil
}

// QAScenarioFingerprint returns a deterministic SHA-256 fingerprint with the scenario name omitted.
func QAScenarioFingerprint(scenario domain.QAScenario) (string, error) {
	payload := struct {
		PublicContractID string          `json:"public_contract_id"`
		Steps            []domain.QAStep `json:"steps"`
	}{PublicContractID: scenario.PublicContractID, Steps: scenario.Steps}
	canonical, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeQAScenarioInput(input qaScenarioInput) (domain.QAScenario, error) {
	scenario := domain.QAScenario{
		Name:             strings.TrimSpace(input.Name),
		PublicContractID: strings.TrimSpace(input.PublicContractID),
		Steps:            make([]domain.QAStep, len(input.Steps)),
	}
	if scenario.Name == "" {
		return domain.QAScenario{}, fmt.Errorf("name is required")
	}
	if scenario.PublicContractID == "" {
		return domain.QAScenario{}, fmt.Errorf("public_contract_id is required")
	}
	if len(input.Steps) == 0 {
		return domain.QAScenario{}, fmt.Errorf("steps must not be empty")
	}
	for i, step := range input.Steps {
		if len(step.Command) == 0 || strings.TrimSpace(step.Command[0]) == "" {
			return domain.QAScenario{}, fmt.Errorf("step %d command must be a non-empty argument vector", i)
		}
		if step.ExpectedExitCode == nil {
			return domain.QAScenario{}, fmt.Errorf("step %d expected_exit_code is required", i)
		}
		command := append([]string(nil), step.Command...)
		stdout := append([]string(nil), step.StdoutContains...)
		if len(stdout) == 0 {
			stdout = nil
		}
		scenario.Steps[i] = domain.QAStep{
			Command:          command,
			Stdin:            step.Stdin,
			ExpectedExitCode: *step.ExpectedExitCode,
			StdoutContains:   stdout,
			StderrPrefix:     step.StderrPrefix,
		}
	}
	return scenario, nil
}

func validateScenarioExpectations(scenario domain.QAScenario, contract domain.PublicContract) error {
	allowedExecutables := stringSet(contract.AllowedExecutables)
	exitCodes := intSet(contract.ExitCodes)
	stdoutValues := stringSet(contract.StdoutContains)
	stderrPrefixes := stringSet(contract.StderrPrefixes)
	for i, step := range scenario.Steps {
		if _, ok := allowedExecutables[step.Command[0]]; !ok {
			return fmt.Errorf("step %d executable %q is not allowed", i, step.Command[0])
		}
		if _, ok := exitCodes[step.ExpectedExitCode]; !ok {
			return fmt.Errorf("step %d exit code %d is not declared by the public contract", i, step.ExpectedExitCode)
		}
		for _, value := range step.StdoutContains {
			if _, ok := stdoutValues[value]; !ok {
				return fmt.Errorf("step %d stdout expectation %q is not declared by the public contract", i, value)
			}
		}
		if step.StderrPrefix != "" {
			if _, ok := stderrPrefixes[step.StderrPrefix]; !ok {
				return fmt.Errorf("step %d stderr prefix %q is not declared by the public contract", i, step.StderrPrefix)
			}
		}
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func intSet(values []int) map[int]struct{} {
	set := make(map[int]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}
