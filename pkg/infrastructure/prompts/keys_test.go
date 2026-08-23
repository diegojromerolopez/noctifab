package prompts

import (
	"strings"
	"testing"
)

func TestCatalog(t *testing.T) {
	t.Run("when listing agents it returns the 6 catalog agents sorted", func(t *testing.T) {
		agents := Agents()
		want := []string{"generator", "planner", "product_manager", "qa", "spec", "tester"}
		if len(agents) != len(want) {
			t.Fatalf("expected %d agents, got %v", len(want), agents)
		}
		for i := range want {
			if agents[i] != want[i] {
				t.Errorf("agent %d: expected %q, got %q", i, want[i], agents[i])
			}
		}
	})

	t.Run("when counting catalog keys it totals 22 actions", func(t *testing.T) {
		total := 0
		for _, agent := range Agents() {
			total += len(Actions(agent))
		}
		if total != 22 {
			t.Fatalf("expected 22 (agent, action) keys, got %d", total)
		}
	})

	t.Run("when validating a known key it succeeds", func(t *testing.T) {
		if err := ValidateKey("tester", "write"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !IsValidKey("generator", "implement_breadth_first_fix") {
			t.Error("expected generator/implement_breadth_first_fix to be valid")
		}
		if !IsValidKey("generator", "surgical_repair") {
			t.Error("expected generator/surgical_repair to be valid")
		}
		if !IsValidKey("qa", "acceptance") {
			t.Error("expected qa/acceptance to be valid")
		}
	})

	t.Run("when validating an unknown agent it fails", func(t *testing.T) {
		if err := ValidateKey("architect", "design"); err == nil {
			t.Fatal("expected error for unknown agent")
		}
		if IsValidKey("qa", "review") {
			t.Error("expected qa/review to be invalid")
		}
	})

	t.Run("when validating an unknown action it fails naming the agent", func(t *testing.T) {
		err := ValidateKey("tester", "nonexistent")
		if err == nil {
			t.Fatal("expected error for unknown action")
		}
	})

	t.Run("when mutating the Actions result it does not affect the catalog", func(t *testing.T) {
		actions := Actions("planner")
		actions[0] = "mutated"
		if Actions("planner")[0] != "decompose" {
			t.Error("catalog was mutated through Actions result")
		}
	})
}

func TestFixtureData(t *testing.T) {
	t.Run("when requesting fixtures per agent it returns the matching struct type", func(t *testing.T) {
		if _, ok := FixtureData("product_manager").(ProductManagerPromptData); !ok {
			t.Error("expected ProductManagerPromptData")
		}
		if _, ok := FixtureData("planner").(PlannerPromptData); !ok {
			t.Error("expected PlannerPromptData")
		}
		if _, ok := FixtureData("tester").(TaskPromptData); !ok {
			t.Error("expected TaskPromptData for tester")
		}
		if _, ok := FixtureData("generator").(TaskPromptData); !ok {
			t.Error("expected TaskPromptData for generator")
		}
		if _, ok := FixtureData("qa").(QAPromptData); !ok {
			t.Error("expected QAPromptData for qa")
		}
	})
}

func TestContract(t *testing.T) {
	t.Run("when reading contracts for all agents they are non-empty and JSON-schema bearing", func(t *testing.T) {
		for _, agent := range Agents() {
			c := Contract(agent)
			if c == "" {
				t.Errorf("empty contract for %s", agent)
			}
			for _, needle := range []string{"Return format", `"reasoning"`, `"actions"`} {
				if !strings.Contains(c, needle) {
					t.Errorf("contract for %s missing %q", agent, needle)
				}
			}
		}
	})

	t.Run("when reading an unknown agent contract it panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Fatal("expected panic for unknown agent contract")
			}
		}()
		Contract("architect")
	})
}
