package llm

import (
	"errors"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/infrastructure/config"
	"github.com/stretchr/testify/assert"
)

func TestRouter_RateLimitTierRotation(t *testing.T) {
	cfg := &config.Config{
		LLM: config.LLMConfig{
			Provider: "mock-primary",
			Model:    "mock-model",
		},
	}
	router := NewResilientLLMRouter(cfg, nil)

	cand1 := RouterCandidate{
		Name:     "gemini-flash",
		Provider: "gemini",
		Model:    "gemini-3.6-flash",
	}
	cand2 := RouterCandidate{
		Name:     "gemini-pro",
		Provider: "gemini",
		Model:    "gemini-3.6-pro",
	}
	cand3 := RouterCandidate{
		Name:     "claude",
		Provider: "anthropic",
		Model:    "claude-sonnet-5",
	}

	// Initially neither candidate is in cooldown
	assert.False(t, router.isCandidateInCooldown(cand1))
	assert.False(t, router.isCandidateInCooldown(cand2))
	assert.False(t, router.isCandidateInCooldown(cand3))

	// Simulate cand1 hitting an HTTP 429 rate limit error
	rateLimitErr := errors.New("HTTP 429: Resource exhausted / rate limit reached")
	router.handleRateLimitRotation(cand1, rateLimitErr)

	// cand1 should now be in cooldown
	assert.True(t, router.isCandidateInCooldown(cand1))

	// cand2 and cand3 should NOT be in cooldown, allowing immediate rotation
	assert.False(t, router.isCandidateInCooldown(cand2))
	assert.False(t, router.isCandidateInCooldown(cand3))
}

func TestRouter_GetShortestCooldownCandidate(t *testing.T) {
	router := NewResilientLLMRouter(nil, nil)

	cand1 := RouterCandidate{Name: "p1", Provider: "prov1"}
	cand2 := RouterCandidate{Name: "p2", Provider: "prov2"}

	now := time.Now()
	router.cooldowns["p1"] = now.Add(10 * time.Second)
	router.cooldowns["p2"] = now.Add(5 * time.Second)

	best, rem := router.getShortestCooldownCandidate([]RouterCandidate{cand1, cand2})
	assert.Equal(t, "p2", best.Name)
	assert.True(t, rem <= 5*time.Second && rem > 0)
}
