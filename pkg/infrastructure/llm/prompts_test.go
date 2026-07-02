package llm

import (
	"strings"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestPromptBuilder_PythonInvariants(t *testing.T) {
	pb := &PromptBuilder{Role: domain.AgentRoleGenerator, DetectedLang: "python"}
	result := pb.Build("write a function")
	for _, inv := range []string{
		"CONCURRENCY & THREADING INVARIANTS",
		"BaseException",
		"KeyboardInterrupt",
		"daemon=True",
		"signal.signal",
	} {
		if !strings.Contains(result, inv) {
			t.Errorf("expected python invariant %q in prompt", inv)
		}
	}
}

func TestPromptBuilder_GoInvariants(t *testing.T) {
	pb := &PromptBuilder{Role: domain.AgentRoleGenerator, DetectedLang: "go"}
	result := pb.Build("write a function")
	for _, inv := range []string{
		"CONCURRENCY & THREADING INVARIANTS",
		"ctx.Done()",
		"sync.WaitGroup",
		"buffered channels",
		"sync.Once",
	} {
		if !strings.Contains(result, inv) {
			t.Errorf("expected go invariant %q in prompt", inv)
		}
	}
}

func TestPromptBuilder_UnknownLang(t *testing.T) {
	pb := &PromptBuilder{Role: domain.AgentRoleGenerator, DetectedLang: ""}
	result := pb.Build("write a function")
	if strings.Contains(result, "CONCURRENCY & THREADING INVARIANTS") {
		t.Error("expected no invariants for unknown language")
	}
	if result != "write a function" {
		t.Errorf("expected original prompt unchanged, got %q", result)
	}
}

func TestPromptBuilder_Idempotent(t *testing.T) {
	pb := &PromptBuilder{Role: domain.AgentRoleGenerator, DetectedLang: "go"}
	original := "do something\n\nCONCURRENCY & THREADING INVARIANTS (Go):\nalready present"
	result := pb.Build(original)
	if result != original {
		t.Error("expected prompt to be unchanged when invariants already present")
	}
}

func TestPromptBuilder_AppendsAfterBlankLine(t *testing.T) {
	pb := &PromptBuilder{Role: domain.AgentRoleGenerator, DetectedLang: "python"}
	result := pb.Build("base prompt")
	if !strings.Contains(result, "\n\n") {
		t.Error("expected invariants separated by blank line")
	}
	parts := strings.Split(result, "\n\n")
	if len(parts) < 2 {
		t.Error("expected at least 2 parts separated by blank line")
	}
}

func TestPromptBuilder_EmptyPrompt(t *testing.T) {
	pb := &PromptBuilder{Role: domain.AgentRoleGenerator, DetectedLang: "python"}
	result := pb.Build("")
	if !strings.Contains(result, "CONCURRENCY & THREADING INVARIANTS") {
		t.Error("expected invariants even with empty prompt")
	}
}
