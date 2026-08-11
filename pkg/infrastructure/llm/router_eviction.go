package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// RoleContextKey is the context key for passing the active agent role.
type RoleContextKey struct{}

// WithRoleContext attaches an agent role name to the context.
func WithRoleContext(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, RoleContextKey{}, role)
}

type stringKey string

const (
	roleStringKey      stringKey = "role"
	agentRoleStringKey stringKey = "agent_role"
)

// GetRoleFromContext retrieves the active agent role name from context.
func GetRoleFromContext(ctx context.Context) string {
	if roleVal := ctx.Value(RoleContextKey{}); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value(roleStringKey); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value(agentRoleStringKey); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value("role"); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	if roleVal := ctx.Value("agent_role"); roleVal != nil {
		if roleStr, ok := roleVal.(string); ok && roleStr != "" {
			return strings.ToLower(roleStr)
		}
	}
	return ""
}

// isEvictionError reports whether an error signals credit exhaustion or auth failure (401/402).
func isEvictionError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrCreditExhausted) {
		return true
	}
	var he *httpError
	if errors.As(err, &he) {
		if he.StatusCode == 401 || he.StatusCode == 402 {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "creditserror") ||
		strings.Contains(msg, "insufficient balance") ||
		strings.Contains(msg, "payment required") ||
		strings.Contains(msg, "credit exhausted") ||
		strings.Contains(msg, "401 unauthorized") ||
		strings.Contains(msg, "402 payment required")
}

// GetEvictedProviders returns a map of candidate names to their eviction details.
func (r *ResilientLLMRouter) GetEvictedProviders() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make(map[string]string)
	now := time.Now()
	for name, until := range r.evictedUntil {
		if now.Before(until) {
			rem := time.Until(until).Round(time.Second)
			reason := r.evictionReasons[name]
			res[name] = fmt.Sprintf("Evicted for %v (Reason: %s)", rem, reason)
		}
	}
	return res
}
