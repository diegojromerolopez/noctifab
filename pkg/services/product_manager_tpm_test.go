package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/diegojromerolopez/noctifab/pkg/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTPMRateLimiter_EstimateTokens(t *testing.T) {
	limiter := services.NewTPMRateLimiter(50000)
	assert.Equal(t, 0, limiter.EstimateTokens(""))
	assert.Equal(t, 25, limiter.EstimateTokens("This is a 100 character test string that simulates an input prompt destined for the LLM provider API."))
}

func TestTPMRateLimiter_UnderBudgetPassesImmediately(t *testing.T) {
	ctx := context.Background()
	limiter := services.NewTPMRateLimiter(100000)

	start := time.Now()
	err := limiter.WaitIfBudgetExceeded(ctx, 1000)
	require.NoError(t, err)
	assert.True(t, time.Since(start) < 50*time.Millisecond)
}

func TestTPMRateLimiter_ExceedBudgetWaits(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Small TPM limit of 100 tokens
	limiter := services.NewTPMRateLimiter(100)
	err := limiter.WaitIfBudgetExceeded(ctx, 80)
	require.NoError(t, err)

	// Second request of 50 tokens exceeds 100 limit, so it waits and hits context deadline
	err = limiter.WaitIfBudgetExceeded(ctx, 50)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}
