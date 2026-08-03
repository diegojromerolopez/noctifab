package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// latestModelServer returns an httptest server that serves an OpenAI-compatible
// /models list and a valid JSON completion envelope. It reports how many times
// the /models endpoint was hit so tests can assert re-resolution behaviour.
func latestModelServer(t *testing.T) (*httptest.Server, *int64) {
	t.Helper()
	var modelCalls int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			atomic.AddInt64(&modelCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"object":"list","data":[{"id":"qwen3.8-max","object":"model"},{"id":"qwen3.7-max","object":"model"}]}`)
			return
		}
		env := `{"reasoning":"ok","actions":[]}`
		payload, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": string(env)}}},
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	t.Cleanup(srv.Close)
	return srv, &modelCalls
}

// TestComplete_LatestAliasDoesNotMutateSharedClient is a regression test for the
// octopus-review finding: resolveLatestModel previously wrote the resolved
// concrete model back into c.Model on the shared Client, so the "latest" alias
// was permanently pinned after the first call (never re-resolved) and the field
// mutation raced under concurrent Complete calls. The Client's Model field must
// stay untouched after Complete returns.
func TestComplete_LatestAliasDoesNotMutateSharedClient(t *testing.T) {
	srv, _ := latestModelServer(t)

	c := &Client{
		Provider:              "opencode",
		Model:                 "latest",
		APIKey:                "testkey",
		URL:                   srv.URL,
		Timeout:               2 * time.Second,
		IdleTimeout:           2 * time.Second,
		Streaming:             false,
		SkipOnCreditExhausted: true,
	}

	for i := 0; i < 3; i++ {
		resp, err := c.Complete(context.Background(), "do the thing")
		if err != nil {
			t.Fatalf("Complete call %d failed: %v", i+1, err)
		}
		if resp == nil {
			t.Fatalf("Complete call %d returned nil response", i+1)
		}
		if c.Model != "latest" {
			t.Fatalf("Complete call %d mutated shared Client.Model to %q; want it left as the original alias %q", i+1, c.Model, "latest")
		}
	}
}

// TestComplete_LatestAliasConcurrent is a smoke test that exercises the shared
// Client under concurrent Complete calls. Run with `go test -race` to surface
// any data race on the Client struct (previously c.Model was written without a
// lock).
func TestComplete_LatestAliasConcurrent(t *testing.T) {
	srv, _ := latestModelServer(t)

	c := &Client{
		Provider:              "opencode",
		Model:                 "latest",
		APIKey:                "testkey",
		URL:                   srv.URL,
		Timeout:               2 * time.Second,
		IdleTimeout:           2 * time.Second,
		Streaming:             false,
		SkipOnCreditExhausted: true,
	}

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := c.Complete(context.Background(), "do the thing"); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("concurrent Complete failed: %v", err)
	}
	if c.Model != "latest" {
		t.Errorf("shared Client.Model mutated to %q after concurrent calls; want %q", c.Model, "latest")
	}
}
