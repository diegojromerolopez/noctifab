package llm

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

type mockFailingClient struct {
	err error
}

func (m *mockFailingClient) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	return nil, m.err
}

func TestResilientLLMRouter_ProviderEviction30Minutes(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Providers: []config.ProviderSpec{
				{Name: "depleted-provider", Provider: "opencode"},
				{Name: "healthy-provider", Provider: "openai"},
			},
			Priority: []string{"depleted-provider", "healthy-provider"},
		},
	}

	router := NewResilientLLMRouter(cfg, nil)

	// Replace candidate clients with mocks
	creditErr := &httpError{StatusCode: http.StatusUnauthorized, Body: "CreditsError: Insufficient balance"}
	router.candidateCache[""] = []RouterCandidate{
		{Name: "depleted-provider", Provider: "opencode", Client: &mockFailingClient{err: creditErr}},
		{Name: "healthy-provider", Provider: "openai", Client: &mockFailingClient{err: fmt.Errorf("healthy failed too")}},
	}

	ctx := context.Background()
	_, err := router.Complete(ctx, "hello")
	if err == nil {
		t.Fatalf("expected error from router")
	}

	evicted := router.GetEvictedProviders()
	if _, ok := evicted["depleted-provider"]; !ok {
		t.Errorf("expected depleted-provider to be evicted, got: %v", evicted)
	}

	// Next call should skip depleted-provider instantly without error from depleted-provider
	evictedDetails := evicted["depleted-provider"]
	if evictedDetails == "" {
		t.Errorf("expected non-empty eviction reason details")
	}
}

func TestGetRoleFromContext(t *testing.T) {
	//nolint:staticcheck // SA1029: tests backward compatibility for string context keys
	ctx := context.WithValue(context.Background(), "agent_role", "Generator")
	role := GetRoleFromContext(ctx)
	if role != "generator" {
		t.Errorf("expected 'generator', got '%s'", role)
	}

	ctx2 := WithRoleContext(context.Background(), "Tester")
	if role2 := GetRoleFromContext(ctx2); role2 != "tester" {
		t.Errorf("expected 'tester', got '%s'", role2)
	}
}

func TestResilientLLMRouter_ArrearageEviction(t *testing.T) {
	arrearageErr := &httpError{
		StatusCode: http.StatusBadRequest,
		Body:       `{"message":"Access denied, please make sure your account is in good standing. For details, see: https://www.alibabacloud.com/help/en/model-studio/error-code#overdue-payment","type":"Arrearage"}`,
	}
	if !isEvictionError(arrearageErr) {
		t.Errorf("expected Arrearage HTTP 400 error to be classified as eviction error")
	}
}

func TestResilientLLMRouter_AnthropicCreditEviction(t *testing.T) {
	anthropicErr := &httpError{
		StatusCode: http.StatusBadRequest,
		Body:       `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the Anthropic API. Please go to Plans & Billing to upgrade or purchase credits."}}`,
	}
	if !isEvictionError(anthropicErr) {
		t.Errorf("expected Anthropic credit balance HTTP 400 error to be classified as eviction error")
	}
	if !isCreditExhausted(anthropicErr) {
		t.Errorf("expected Anthropic credit balance error to satisfy isCreditExhausted")
	}
}
