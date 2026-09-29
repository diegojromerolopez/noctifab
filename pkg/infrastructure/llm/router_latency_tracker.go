package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// CandidateLatencyStats tracks latency history and timeout events for a named candidate.
type CandidateLatencyStats struct {
	ConsecutiveTimeouts int
	TotalTimeouts       int
	TotalSuccesses      int
	LastDuration        time.Duration
	PenaltyScore        int // 0 = no demotion, 1 = demote to 2nd position, >=2 = demote to 3rd position
	LastUpdated         time.Time
}

// LatencyTracker manages dynamic candidate priority adjustments based on observed latency and timeout rates.
type LatencyTracker struct {
	mu    sync.RWMutex
	stats map[string]*CandidateLatencyStats
}

// NewLatencyTracker creates a new LatencyTracker.
func NewLatencyTracker() *LatencyTracker {
	return &LatencyTracker{
		stats: make(map[string]*CandidateLatencyStats),
	}
}

// RecordOutcome updates latency statistics for a candidate.
func (lt *LatencyTracker) RecordOutcome(candidateName string, duration time.Duration, err error, timeoutThreshold time.Duration) {
	if candidateName == "" {
		return
	}
	lt.mu.Lock()
	defer lt.mu.Unlock()

	s, exists := lt.stats[candidateName]
	if !exists {
		s = &CandidateLatencyStats{}
		lt.stats[candidateName] = s
	}

	s.LastDuration = duration
	s.LastUpdated = time.Now()

	isTimeout := isTimeoutOrCanceled(err)
	if !isTimeout && timeoutThreshold > 0 && duration >= timeoutThreshold {
		isTimeout = true
	}

	if isTimeout {
		s.ConsecutiveTimeouts++
		s.TotalTimeouts++
		// Increase penalty: 1 timeout -> penalty 1 (2nd pos), >=2 timeouts -> penalty 2+ (3rd pos)
		s.PenaltyScore++
		if s.PenaltyScore > 3 {
			s.PenaltyScore = 3
		}
	} else if err == nil {
		s.TotalSuccesses++
		s.ConsecutiveTimeouts = 0
		if duration > 60*time.Second {
			// High latency penalty: if a single successful response takes > 60s, assign penalty 1
			if s.PenaltyScore < 1 {
				s.PenaltyScore = 1
			}
		} else if duration < 25*time.Second {
			// Fast response gradually clears penalty score
			if s.PenaltyScore > 0 {
				s.PenaltyScore--
			}
		}
	}
}

// GetPenaltyScore returns the current penalty score for a candidate.
func (lt *LatencyTracker) GetPenaltyScore(candidateName string) int {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	if s, ok := lt.stats[candidateName]; ok {
		return s.PenaltyScore
	}
	return 0
}

// GetConsecutiveTimeouts returns the consecutive timeouts count for a candidate.
func (lt *LatencyTracker) GetConsecutiveTimeouts(candidateName string) int {
	lt.mu.RLock()
	defer lt.mu.RUnlock()
	if s, ok := lt.stats[candidateName]; ok {
		return s.ConsecutiveTimeouts
	}
	return 0
}

// ApplyDynamicDemotion reorders the candidates list based on observed timeout/latency penalties.
// A candidate with PenaltyScore == 1 is demoted by 1 position (to 2nd place).
// A candidate with PenaltyScore >= 2 is demoted by 2 positions (to 3rd place).
func (lt *LatencyTracker) ApplyDynamicDemotion(candidates []RouterCandidate) []RouterCandidate {
	if len(candidates) <= 1 {
		return candidates
	}

	lt.mu.RLock()
	defer lt.mu.RUnlock()

	result := make([]RouterCandidate, len(candidates))
	copy(result, candidates)

	processed := make(map[string]bool)

	// Evaluate candidates from first to last
	for i := 0; i < len(result); i++ {
		name := result[i].Name
		if processed[name] {
			continue
		}
		processed[name] = true

		s, found := lt.stats[name]
		if !found || s.PenaltyScore <= 0 {
			continue
		}

		targetPos := i + s.PenaltyScore
		if targetPos >= len(result) {
			targetPos = len(result) - 1
		}

		if targetPos > i {
			fmt.Fprintf(os.Stderr, "ℹ [LLM Router Demotion] Candidate '%s' dynamically demoted from position %d to %d (penalty=%d, timeouts=%d, last=%v)\n",
				name, i+1, targetPos+1, s.PenaltyScore, s.ConsecutiveTimeouts, s.LastDuration.Round(time.Millisecond))
			// Shift elements to demote candidate to targetPos
			demoted := result[i]
			copy(result[i:targetPos], result[i+1:targetPos+1])
			result[targetPos] = demoted
		}
	}

	return result
}

// isTimeoutOrCanceled returns true if the error indicates a context deadline or cancellation.
func isTimeoutOrCanceled(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "timed out")
}
