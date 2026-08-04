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

// providerClient builds the ProviderClient for this Client's provider using
// the data-driven ProviderSpec dispatch (no protocol switch statements).
func (c *Client) providerClient() ProviderClient {
	spec, _ := GetProviderSpec(strings.ToLower(c.Provider))
	if spec != nil && spec.NewClientFunc != nil {
		return spec.NewClientFunc(c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
	}
	return NewOpenAIProviderClient(c.Provider, c.URL, c.Timeout, c.IdleTimeout, c.Streaming)
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
	if entry, ok := c.catalogCache[key]; ok && time.Now().Before(entry.expires) {
		c.catalogMu.Unlock()
		return entry.models, nil
	}
	c.catalogMu.Unlock()

	models, err := pClient.GetAvailableModels(ctx, apiKey)
	if err != nil || len(models) == 0 {
		return models, err
	}

	c.catalogMu.Lock()
	if c.catalogCache == nil {
		c.catalogCache = make(map[string]catalogEntry)
	}
	c.catalogCache[key] = catalogEntry{models: models, expires: time.Now().Add(ttl)}
	c.catalogMu.Unlock()
	return models, nil
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

	return selectLowerModelFromParsed(currentModel, parsedModels)
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

	alias := normalizeModelID(c.Model)
	if alias != "" && alias != "latest" && alias != "auto" {
		// If the alias itself is a concrete, pinned model in the provider's
		// catalog, use it verbatim — no resolution needed. A model counts as
		// concrete only when it is NOT a moving alias: OpenRouter's `~`-pinned
		// latest pointers (e.g. `~deepseek/deepseek-v4-flash-latest`) route to
		// variable upstreams and are exactly what we want to resolve away.
		for _, m := range available {
			norm := normalizeModelID(m)
			if norm == alias && !isMovingAlias(m) {
				return m
			}
		}

		// Scope resolution to the alias's own model family. Aggregators like
		// OpenRouter expose thousands of unrelated models; ranking them all
		// globally could resolve a `deepseek/*-latest` alias to an unrelated
		// top-ranked model (e.g. sao10k/l3-lunaris-8b). Prefer models matching
		// the alias base family before falling back to the provider-wide best.
		if family := filterModelsForAlias(alias, parsedModels); len(family) > 0 {
			sortProviderModels(family)
			return family[0].Name
		}
	}

	sortProviderModels(parsedModels)
	return parsedModels[0].Name
}
