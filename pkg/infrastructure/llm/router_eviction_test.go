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
