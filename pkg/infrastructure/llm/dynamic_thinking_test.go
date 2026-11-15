package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

func TestIsRoutineTask(t *testing.T) {
	tests := []struct {
		role     string
		expected bool
	}{
		{"generator", true},
		{"generators", true},
		{"tester", true},
		{"testers", true},
		{"spike", true},
		{"planner", false},
		{"auditor", false},
		{"product_manager", false},
		{"orchestrator", false},
		{"fallback", false},
		{"", false},
	}

	for _, tt := range tests {
		got := isRoutineTask(tt.role)
		if got != tt.expected {
			t.Errorf("isRoutineTask(%q) = %v, expected %v", tt.role, got, tt.expected)
		}
	}
}

func TestParseDynamicModelCapabilities(t *testing.T) {
	// Raw JSON returned dynamically by /models endpoint with no hardcoded names
	rawJSON := `[
		{
			"id": "dynamic-custom-model-alpha",
			"supported_parameters": ["max_tokens", "temperature", "reasoning_effort"]
		},
		{
			"id": "dynamic-fast-model-beta",
			"supported_parameters": ["max_tokens", "temperature"]
		},
		{
			"id": "dynamic-thinker-gamma",
			"capabilities": {
				"thinking": true
			}
		},
		{
			"name": "models/dynamic-gemini-style",
			"thinkingConfig": {
				"supported": true
			}
		},
		{
			"id": "dynamic-arch-reasoner",
			"architecture": {
				"instruct_type": "reasoning"
			}
		}
	]`

	var rawModels []map[string]interface{}
	if err := json.Unmarshal([]byte(rawJSON), &rawModels); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	caps := parseDynamicModelCapabilities(rawModels)

	if !caps["dynamic-custom-model-alpha"].SupportsThinking {
		t.Errorf("expected dynamic-custom-model-alpha to support thinking via reasoning_effort")
	}
	if caps["dynamic-fast-model-beta"].SupportsThinking {
		t.Errorf("expected dynamic-fast-model-beta to NOT support thinking")
	}
	if !caps["dynamic-thinker-gamma"].SupportsThinking {
		t.Errorf("expected dynamic-thinker-gamma to support thinking via capabilities.thinking")
	}
	if !caps["dynamic-gemini-style"].SupportsThinking {
		t.Errorf("expected dynamic-gemini-style to support thinking via thinkingConfig")
	}
	if !caps["dynamic-arch-reasoner"].SupportsThinking {
		t.Errorf("expected dynamic-arch-reasoner to support thinking via architecture.instruct_type")
	}
}

func TestAdjustForRoutineTask(t *testing.T) {
	// Setup a mock HTTP server simulating a provider's /models endpoint
	modelsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": "model-xyz-thinking",
					"supported_parameters": []interface{}{
						"temperature",
						"reasoning",
					},
				},
				{
					"id": "model-xyz-standard",
					"supported_parameters": []interface{}{
						"temperature",
					},
				},
				{
					"id": "gemini-3.1-pro-preview",
					"supported_parameters": []interface{}{
						"temperature",
						"reasoning",
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer modelsServer.Close()

	client := &Client{
		Provider: "openai",
		URL:      modelsServer.URL,
		APIKeys:  []string{"test-token"},
	}

	// Case 1: Routine task ("generator") with a model supporting thinking
	ctxGen := domain.WithRoleContext(context.Background(), "generator")
	extra, modified := client.adjustForRoutineTask(ctxGen, "generator", "model-xyz-thinking", nil)
	if !modified {
		t.Fatalf("expected modified = true for generator on model-xyz-thinking")
	}
	if extra["enable_thinking"] != false {
		t.Errorf("expected enable_thinking = false, got %v", extra["enable_thinking"])
	}
	if extra["thinking_budget"] != 0 {
		t.Errorf("expected thinking_budget = 0, got %v", extra["thinking_budget"])
	}
	if extra["reasoning_effort"] != "low" {
		t.Errorf("expected reasoning_effort = 'low', got %v", extra["reasoning_effort"])
	}

	// Case 2: Routine task ("tester") with a model supporting thinking
	ctxTester := domain.WithRoleContext(context.Background(), "tester")
	extraTester, modifiedTester := client.adjustForRoutineTask(ctxTester, "tester", "model-xyz-thinking", nil)
	if !modifiedTester {
		t.Fatalf("expected modified = true for tester on model-xyz-thinking")
	}
	if extraTester["enable_thinking"] != false {
		t.Errorf("expected enable_thinking = false for tester")
	}

	// Case 3: Routine task ("spike") with a model supporting thinking
	ctxSpike := domain.WithRoleContext(context.Background(), "spike")
	extraSpike, modifiedSpike := client.adjustForRoutineTask(ctxSpike, "spike", "model-xyz-thinking", nil)
	if !modifiedSpike {
		t.Fatalf("expected modified = true for spike on model-xyz-thinking")
	}
	if extraSpike["enable_thinking"] != false {
		t.Errorf("expected enable_thinking = false for spike")
	}

	// Case 4: Non-routine task ("planner") on model supporting thinking -> NOT suppressed
	ctxPlanner := domain.WithRoleContext(context.Background(), "planner")
	extraPlanner, modifiedPlanner := client.adjustForRoutineTask(ctxPlanner, "planner", "model-xyz-thinking", nil)
	if modifiedPlanner {
		t.Fatalf("expected modified = false for planner, but was modified: %v", extraPlanner)
	}

	// Case 5: Routine task ("generator") on standard model without thinking -> NOT modified
	_, modifiedStd := client.adjustForRoutineTask(ctxGen, "generator", "model-xyz-standard", nil)
	if modifiedStd {
		t.Fatalf("expected modified = false for model-xyz-standard without thinking capability")
	}

	// Case 6: Routine task ("generator") on thinking-only model -> NOT modified because thinking cannot be disabled
	_, modifiedThinkingOnly := client.adjustForRoutineTask(ctxGen, "generator", "gemini-3.1-pro-preview", nil)
	if modifiedThinkingOnly {
		t.Fatalf("expected modified = false for thinking-only model where thinking cannot be disabled")
	}

	// Case 7: Explicit EnableThinking = false on client bypasses capabilities lookup and disables thinking
	disabled := false
	clientDisabled := &Client{
		Provider:       "openai",
		EnableThinking: &disabled,
	}
	extraDisabled, modifiedDisabled := clientDisabled.adjustForRoutineTask(ctxGen, "generator", "any-model", nil)
	if !modifiedDisabled {
		t.Fatalf("expected modified = true when EnableThinking is explicitly false")
	}
	if extraDisabled["enable_thinking"] != false || extraDisabled["thinking_budget"] != 0 {
		t.Errorf("expected thinking disabled fields in extra, got %v", extraDisabled)
	}
}

func TestExtraBodySetterClients(t *testing.T) {
	// Verify OpenAI Client implements extraBodySetter
	openAI := NewOpenAIClient("https://api.openai.com/v1", 0, 0, false)
	if setter, ok := openAI.(extraBodySetter); !ok {
		t.Errorf("OpenAIClient does not implement extraBodySetter")
	} else {
		setter.SetExtraBody(map[string]interface{}{"thinking_budget": 0})
	}

	// Verify Anthropic Client implements extraBodySetter
	anthropic := NewAnthropicProviderClient("https://api.anthropic.com/v1", 0, 0, false)
	if setter, ok := anthropic.(extraBodySetter); !ok {
		t.Errorf("AnthropicClient does not implement extraBodySetter")
	} else {
		setter.SetExtraBody(map[string]interface{}{"thinking_budget": 0})
	}

	// Verify Gemini Client implements extraBodySetter
	gemini := NewGeminiProviderClient("https://generativelanguage.googleapis.com", 0, 0, false)
	if setter, ok := gemini.(extraBodySetter); !ok {
		t.Errorf("GeminiClient does not implement extraBodySetter")
	} else {
		setter.SetExtraBody(map[string]interface{}{"thinking_budget": 0})
	}
}

func TestOpenAIFiltersThinkingExtraBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}
		if _, exists := reqBody["thinking_budget"]; exists {
			t.Errorf("expected thinking_budget to be filtered for OpenAI, but found it in request body")
		}
		if _, exists := reqBody["enable_thinking"]; exists {
			t.Errorf("expected enable_thinking to be filtered for OpenAI, but found it in request body")
		}
		if _, exists := reqBody["thinkingConfig"]; exists {
			t.Errorf("expected thinkingConfig to be filtered for OpenAI, but found it in request body")
		}
		if _, exists := reqBody["thinking"]; exists {
			t.Errorf("expected thinking to be filtered for OpenAI, but found it in request body")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id": "test-123",
			"choices": []map[string]interface{}{
				{
					"message": map[string]string{
						"role":    "assistant",
						"content": `{"actions":[]}`,
					},
				},
			},
		})
	}))
	defer server.Close()

	client := NewOpenAIClient(server.URL, 5*time.Second, 5*time.Second, false)
	if setter, ok := client.(extraBodySetter); ok {
		setter.SetExtraBody(map[string]interface{}{
			"thinking_budget": 0,
			"enable_thinking": false,
			"thinkingConfig":  map[string]interface{}{"thinkingBudget": 0},
			"thinking":        map[string]interface{}{"type": "disabled"},
		})
	}
	_, err := client.Call(context.Background(), "gpt-5.6-luna", "test-key", "json test", 100, 0.7)
	if err != nil {
		t.Fatalf("unexpected error calling client: %v", err)
	}
}
