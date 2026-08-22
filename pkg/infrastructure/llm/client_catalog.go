package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// defaultCatalogTTL bounds how long a provider's model catalog is reused
// before GetAvailableModels is queried again.
const defaultCatalogTTL = 5 * time.Minute

// catalogEntry is a cached model catalog with its expiry deadline.
type catalogEntry struct {
	models  []string
	expires time.Time
}

// extraBodySetter is an optional interface that provider clients can implement
// to receive provider-specific extra body parameters after construction.
type extraBodySetter interface {
	SetExtraBody(params map[string]interface{})
}

// disableJSONModeSetter is an optional interface that provider clients can
// implement to disable response_format=json_object for a given provider.
type disableJSONModeSetter interface {
	SetDisableJSONMode(v bool)
}

// providerClient builds the ProviderClient for this Client's provider using
// the data-driven ProviderSpec dispatch (no protocol switch statements).
func (c *Client) providerClient() ProviderClient {
	spec, _ := GetProviderSpec(strings.ToLower(c.Provider))
	var pc ProviderClient
	if spec != nil && spec.NewClientFunc != nil {
		pc = spec.NewClientFunc(c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
	} else {
		pc = NewOpenAIProviderClient(c.Provider, c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
	}
	// Inject provider-specific extra body parameters (e.g. enable_thinking, thinking_budget)
	// if the client supports the extraBodySetter interface.
	extra := make(map[string]interface{})
	if c.EnableThinking != nil {
		extra["enable_thinking"] = *c.EnableThinking
	}
	if c.ThinkingBudget != nil {
		extra["thinking_budget"] = *c.ThinkingBudget
	}
	for k, v := range c.ExtraParams {
		if _, exists := extra[k]; !exists {
			switch strings.ToLower(v) {
			case "true":
				extra[k] = true
			case "false":
				extra[k] = false
			default:
				extra[k] = v
			}
		}
	}
	if len(extra) > 0 {
		if setter, ok := pc.(extraBodySetter); ok {
			setter.SetExtraBody(extra)
		}
	}

	// Propagate DisableJSONMode so the client skips response_format=json_object.
	// Automatic when thinking is enabled (thinking trace in response breaks forced JSON mode).
	thinkingOn := c.EnableThinking != nil && *c.EnableThinking
	if c.DisableJSONMode || thinkingOn {
		if setter, ok := pc.(disableJSONModeSetter); ok {
			setter.SetDisableJSONMode(true)
		}
	}
	return pc
}

// catalogCacheKey identifies a catalog by provider and base URL only. The
// API key is intentionally excluded so the key can never leak a secret into
// logs or debug dumps.
func (c *Client) catalogCacheKey() string {
	return strings.ToLower(c.Provider) + "|" + c.URL
}

// availableModelsCached returns the provider's model catalog, serving it
// from the TTL cache when a fresh entry exists. Errors and empty catalogs
// are never cached so transient lookup failures self-heal on the next call.
func (c *Client) availableModelsCached(ctx context.Context, pClient ProviderClient, apiKey string) ([]string, error) {
	key := c.catalogCacheKey()
	ttl := c.catalogTTL
	if ttl <= 0 {
		ttl = defaultCatalogTTL
	}

	c.catalogMu.Lock()
	entry, ok := c.catalogCache[key]
	if ok && len(entry.models) > 0 {
		now := time.Now()
		// Serve from cache instantly. If expiring soon (< 20% TTL) or expired, refresh asynchronously in background.
		if entry.expires.Sub(now) < ttl/5 {
			go c.refreshCatalogAsync(pClient, apiKey)
		}
		c.catalogMu.Unlock()
		return entry.models, nil
	}
	c.catalogMu.Unlock()

	return c.fetchAndCacheCatalog(ctx, pClient, apiKey, ttl)
}

func (c *Client) fetchAndCacheCatalog(ctx context.Context, pClient ProviderClient, apiKey string, ttl time.Duration) ([]string, error) {
	models, err := pClient.GetAvailableModels(ctx, apiKey)
	if err != nil || len(models) == 0 {
		return models, err
	}

	key := c.catalogCacheKey()
	c.catalogMu.Lock()
	if c.catalogCache == nil {
		c.catalogCache = make(map[string]catalogEntry)
	}
	c.catalogCache[key] = catalogEntry{models: models, expires: time.Now().Add(ttl)}
	c.catalogMu.Unlock()
	return models, nil
}

func (c *Client) refreshCatalogAsync(pClient ProviderClient, apiKey string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ttl := c.catalogTTL
	if ttl <= 0 {
		ttl = defaultCatalogTTL
	}
	_, _ = c.fetchAndCacheCatalog(ctx, pClient, apiKey, ttl)
}

// clearCatalogCache drops all cached catalogs. Exposed for tests.
func (c *Client) clearCatalogCache() {
	c.catalogMu.Lock()
	c.catalogCache = nil
	c.catalogMu.Unlock()
}

// parseCatalogModels runs the provider's model parser over a raw catalog.
func (c *Client) parseCatalogModels(available []string) []*ProviderModelInfo {
	spec, _ := GetProviderSpec(strings.ToLower(c.Provider))
	parser := parseOpenAIModel
	if spec != nil && spec.ParseModelFunc != nil {
		parser = spec.ParseModelFunc
	}
	var parsedModels []*ProviderModelInfo
	for _, m := range available {
		if info, parsed := parser(m); parsed && info != nil {
			parsedModels = append(parsedModels, info)
		}
	}
	return parsedModels
}

func (c *Client) getNextLowerModel(ctx context.Context, apiKey, currentModel string) string {
	pClient := c.providerClient()

	available, err := c.availableModelsCached(ctx, pClient, apiKey)
	if err != nil || len(available) == 0 {
		fmt.Fprintf(os.Stderr, "⚠ Warning: failed to query available models from %s API endpoint: %v\n", c.Provider, err)
		return ""
	}

	parsedModels := c.parseCatalogModels(available)
	if len(parsedModels) == 0 {
		return ""
	}

	return selectFallbackModel(c.Model, currentModel, parsedModels)
}

func (c *Client) resolveLatestModel(ctx context.Context, apiKey string) string {
	pClient := c.providerClient()

	available, err := c.availableModelsCached(ctx, pClient, apiKey)
	if err != nil || len(available) == 0 {
		return ""
	}

	parsedModels := c.parseCatalogModels(available)
	if len(parsedModels) == 0 {
		return ""
	}

	sortProviderModels(parsedModels)
	return parsedModels[0].Name
}
