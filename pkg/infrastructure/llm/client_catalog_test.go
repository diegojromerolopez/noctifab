package llm

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingProviderClient counts GetAvailableModels calls.
type countingProviderClient struct {
	ProviderClient
	models []string
	calls  int
}

func (c *countingProviderClient) GetAvailableModels(_ context.Context, _ string) ([]string, error) {
	c.calls++
	return c.models, nil
}

func TestCatalogCache(t *testing.T) {
	t.Run("when the catalog is queried twice within the TTL it hits the provider only once", func(t *testing.T) {
		client := &Client{Provider: "openai", Model: "gpt-4o", catalogTTL: time.Minute}
		pc := &countingProviderClient{models: []string{"gpt-4o", "gpt-4o-mini"}}

		first, err := client.availableModelsCached(context.Background(), pc, "key")
		require.NoError(t, err)
		second, err := client.availableModelsCached(context.Background(), pc, "key")
		require.NoError(t, err)

		assert.Equal(t, first, second)
		assert.Equal(t, 1, pc.calls, "second lookup within TTL must be served from cache")
	})

	t.Run("when the TTL has expired it re-queries the provider", func(t *testing.T) {
		client := &Client{Provider: "openai", Model: "gpt-4o", catalogTTL: time.Nanosecond}
		pc := &countingProviderClient{models: []string{"gpt-4o"}}

		_, err := client.availableModelsCached(context.Background(), pc, "key")
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)
		_, err = client.availableModelsCached(context.Background(), pc, "key")
		require.NoError(t, err)
		time.Sleep(50 * time.Millisecond) // Allow background async refresh to complete

		assert.Equal(t, 2, pc.calls)
	})

	t.Run("when the cache is cleared it re-queries the provider", func(t *testing.T) {
		client := &Client{Provider: "openai", Model: "gpt-4o", catalogTTL: time.Minute}
		pc := &countingProviderClient{models: []string{"gpt-4o"}}

		_, err := client.availableModelsCached(context.Background(), pc, "key")
		require.NoError(t, err)
		client.clearCatalogCache()
		_, err = client.availableModelsCached(context.Background(), pc, "key")
		require.NoError(t, err)

		assert.Equal(t, 2, pc.calls)
	})

	t.Run("when the provider returns an empty catalog it is not cached", func(t *testing.T) {
		client := &Client{Provider: "openai", Model: "gpt-4o", catalogTTL: time.Minute}
		pc := &countingProviderClient{models: nil}

		_, err := client.availableModelsCached(context.Background(), pc, "key")
		require.NoError(t, err)
		_, err = client.availableModelsCached(context.Background(), pc, "key")
		require.NoError(t, err)

		assert.Equal(t, 2, pc.calls, "empty catalogs must not be cached so failures self-heal")
	})
}
