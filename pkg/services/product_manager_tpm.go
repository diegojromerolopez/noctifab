package services

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// DefaultTPMLimit is a conservative token-per-minute threshold for tier 1 / tier 2 LLM providers.
const DefaultTPMLimit = 60000

// TPMRateLimiter enforces token-per-minute budgeting across concurrent HTTP requests.
type TPMRateLimiter struct {
	mu           sync.Mutex
	tpmLimit     int
	tokenEntries []tokenEntry
}

type tokenEntry struct {
	timestamp time.Time
	tokens    int
}

// NewTPMRateLimiter creates a new rate limiter with the given TPM limit.
func NewTPMRateLimiter(tpmLimit int) *TPMRateLimiter {
	if tpmLimit <= 0 {
		tpmLimit = DefaultTPMLimit
	}
	return &TPMRateLimiter{
		tpmLimit: tpmLimit,
	}
}

// EstimateTokens calculates an approximate token count based on string length (heuristic ~4 chars/token).
func (l *TPMRateLimiter) EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	tokens := len(text) / 4
	if tokens == 0 {
		tokens = 1
	}
	return tokens
}

// WaitIfBudgetExceeded checks if adding estimatedTokens would breach the 1-minute rolling TPM budget.
// If it would breach the budget, it pauses execution until enough tokens expire from the rolling window.
func (l *TPMRateLimiter) WaitIfBudgetExceeded(ctx context.Context, estimatedTokens int) error {
	for {
		l.mu.Lock()
		now := time.Now()
		cutoff := now.Add(-1 * time.Minute)

		// Prune entries older than 1 minute
		validEntries := l.tokenEntries[:0]
		currentTokens := 0
		for _, e := range l.tokenEntries {
			if e.timestamp.After(cutoff) {
				validEntries = append(validEntries, e)
				currentTokens += e.tokens
			}
		}
		l.tokenEntries = validEntries

		// If adding estimatedTokens fits within TPM limit, record and proceed
		if currentTokens+estimatedTokens <= l.tpmLimit || len(l.tokenEntries) == 0 {
			l.tokenEntries = append(l.tokenEntries, tokenEntry{
				timestamp: now,
				tokens:    estimatedTokens,
			})
			l.mu.Unlock()
			return nil
		}

		// Calculate wait duration until oldest entry rolls past 1 minute
		oldest := l.tokenEntries[0].timestamp
		waitDuration := oldest.Add(1 * time.Minute).Sub(now)
		if waitDuration <= 0 {
			waitDuration = 50 * time.Millisecond
		}
		l.mu.Unlock()

		fmt.Printf("⚡ [TPM Rate Limiter] Projected tokens (%d, current=%d) exceed TPM limit (%d). Pausing %v to prevent provider 429...\n",
			estimatedTokens, currentTokens, l.tpmLimit, waitDuration.Round(time.Millisecond))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			// Loop again to re-check
		}
	}
}
