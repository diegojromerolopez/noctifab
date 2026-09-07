package ensemble

import (
	"context"
	"fmt"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
)

// NamedClient pairs a provider configuration name with its domain.LLMClient implementation.
type NamedClient struct {
	Name      string
	Client    domain.LLMClient
	MaxTokens *int
}

// StageClient defines a client assigned to an ensembled pipeline stage.
type StageClient struct {
	Name             string
	Client           domain.LLMClient
	RefinementPrompt string
	MaxTokens        *int
}

// TargetClient defines a specialist client in a divide-and-conquer strategy.
type TargetClient struct {
	Name        string
	Client      domain.LLMClient
	RolePrompt  string
	TargetFiles []string
	MaxTokens   *int
}

// Client represents a unified ensembled LLM client implementing domain.LLMClient.
type Client struct {
	strategy config.EnsembleStrategy
	delegate domain.LLMClient
}

// NewClient creates a new unified ensemble client wrapping the resolved strategy delegate.
func NewClient(strategy config.EnsembleStrategy, delegate domain.LLMClient) *Client {
	return &Client{
		strategy: strategy,
		delegate: delegate,
	}
}

// Complete delegates completion to the configured multi-model strategy.
func (c *Client) Complete(ctx context.Context, prompt string) (*domain.LLMResponse, error) {
	if c.delegate == nil {
		return nil, fmt.Errorf("ensemble client has no strategy delegate configured (strategy=%s)", c.strategy)
	}
	return c.delegate.Complete(ctx, prompt)
}

// Strategy returns the configured ensemble strategy.
func (c *Client) Strategy() config.EnsembleStrategy {
	return c.strategy
}
