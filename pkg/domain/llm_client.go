package domain

import (
	"context"
	"strings"
	"sync"
	"time"
)

// LLMAction represents a specific tool call request produced by the LLM.
type LLMAction struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

// TokenUsage encapsulates authoritative token metrics from an LLM completion.
type TokenUsage struct {
	InputTokens     int64 `json:"input_tokens"`
	OutputTokens    int64 `json:"output_tokens"`
	ReasoningTokens int64 `json:"reasoning_tokens,omitempty"` // Included in OutputTokens
	CachedTokens    int64 `json:"cached_tokens,omitempty"`    // Included in InputTokens
	TotalTokens     int64 `json:"total_tokens"`
}

// LLMResponse is the structured schema returned by the LLM client.
type LLMResponse struct {
	Reasoning string      `json:"reasoning"`
	Actions   []LLMAction `json:"actions"`
	Usage     TokenUsage  `json:"usage"`
}

// LLMClient defines the interface for communicating with an external AI provider.
type LLMClient interface {
	// Complete generates a completion for the given system/user prompt.
	Complete(ctx context.Context, prompt string) (*LLMResponse, error)
}

// uncompactableTailKey is the context key carrying the byte length of the
// non-compactable tail at the end of a prompt (the machine-readable output
// contract appended by the prompts renderer). Prompt compaction must never
// rewrite that block, so the LLM client compacts only the bytes before it.
type uncompactableTailKey struct{}

// WithUncompactableTail marks the last tailLen bytes of the prompt sent with
// ctx as non-compactable (the output contract block).
func WithUncompactableTail(ctx context.Context, tailLen int) context.Context {
	return context.WithValue(ctx, uncompactableTailKey{}, tailLen)
}

// UncompactableTailLen returns the non-compactable tail length recorded in
// ctx, or 0 when none was set.
func UncompactableTailLen(ctx context.Context) int {
	if n, ok := ctx.Value(uncompactableTailKey{}).(int); ok && n > 0 {
		return n
	}
	return 0
}

// cacheablePrefixKey carries the byte length of the static cacheable prefix
// at the beginning of a prompt (e.g. system instructions and static body).
type cacheablePrefixKey struct{}

// WithCacheablePrefix marks the first prefixLen bytes of the prompt sent with
// ctx as cacheable static prefix, enabling structured multi-block prompt caching.
func WithCacheablePrefix(ctx context.Context, prefixLen int) context.Context {
	return context.WithValue(ctx, cacheablePrefixKey{}, prefixLen)
}

// CacheablePrefixLen returns the cacheable static prefix length recorded in
// ctx, or 0 when none was set.
func CacheablePrefixLen(ctx context.Context) int {
	if n, ok := ctx.Value(cacheablePrefixKey{}).(int); ok && n > 0 {
		return n
	}
	return 0
}

// cacheSessionIDKey carries an application session or task identifier to
// route related LLM requests to warm cache nodes.
type cacheSessionIDKey struct{}

// WithCacheSessionID attaches a stable session/task ID to ctx for routing affinity.
func WithCacheSessionID(ctx context.Context, sessionID string) context.Context {
	return context.WithValue(ctx, cacheSessionIDKey{}, sessionID)
}

// CacheSessionID returns the session ID recorded in ctx, or "" when none was set.
func CacheSessionID(ctx context.Context) string {
	if s, ok := ctx.Value(cacheSessionIDKey{}).(string); ok && s != "" {
		return s
	}
	return ""
}

// RoleContextKey is the typed context key for passing the active agent role.
type RoleContextKey struct{}

// WithRoleContext attaches an agent role name to the context.
func WithRoleContext(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, RoleContextKey{}, role)
}

// GetRoleFromContext retrieves the active agent role name from context.
func GetRoleFromContext(ctx context.Context) string {
	if roleVal := ctx.Value(RoleContextKey{}); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value("agent_role"); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value("role"); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	return ""
}

// StreamLivenessTracker tracks real-time streaming chunk arrivals to prevent
// redundant speculative hedging when a primary candidate is actively streaming tokens.
type StreamLivenessTracker struct {
	mu           sync.Mutex
	started      bool
	chunkCount   int
	lastActivity time.Time
}

// NewStreamLivenessTracker creates a new StreamLivenessTracker.
func NewStreamLivenessTracker() *StreamLivenessTracker {
	return &StreamLivenessTracker{}
}

// RecordChunk records the arrival of a streaming data chunk.
func (s *StreamLivenessTracker) RecordChunk() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.started = true
	s.chunkCount++
	s.lastActivity = time.Now()
}

// IsActive returns whether chunks have been received at or after since.
func (s *StreamLivenessTracker) IsActive(since time.Time) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started && (s.lastActivity.After(since) || s.lastActivity.Equal(since))
}

// ChunkCount returns the total number of chunks received so far.
func (s *StreamLivenessTracker) ChunkCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chunkCount
}

type streamLivenessKey struct{}

// WithStreamLivenessTracker attaches a StreamLivenessTracker to the context.
func WithStreamLivenessTracker(ctx context.Context, tracker *StreamLivenessTracker) context.Context {
	return context.WithValue(ctx, streamLivenessKey{}, tracker)
}

// StreamLivenessTrackerFromContext retrieves the StreamLivenessTracker from context, or nil.
func StreamLivenessTrackerFromContext(ctx context.Context) *StreamLivenessTracker {
	if tracker, ok := ctx.Value(streamLivenessKey{}).(*StreamLivenessTracker); ok {
		return tracker
	}
	return nil
}

type auditModeKey struct{}

// WithAuditMode attaches an audit mode (e.g. "smart" or "exhaustive") to the context.
func WithAuditMode(ctx context.Context, mode string) context.Context {
	return context.WithValue(ctx, auditModeKey{}, mode)
}

// AuditModeFromContext retrieves the audit mode from context (default: "smart").
func AuditModeFromContext(ctx context.Context) string {
	if mode, ok := ctx.Value(auditModeKey{}).(string); ok && mode != "" {
		return strings.ToLower(strings.TrimSpace(mode))
	}
	if mode, ok := ctx.Value("audit_mode").(string); ok && mode != "" {
		return strings.ToLower(strings.TrimSpace(mode))
	}
	return "smart"
}
