# Developer Guide

This guide is intended for software engineers looking to extend, modify, or test the `noctifab` orchestration engine. It details repository guidelines, testing strategies, security configurations, and coding standards.

---

## Coding Constraints

To maintain a highly modular, clean, and AI-agent-friendly codebase, all contributions must strictly adhere to the following constraints:

### 1. File Size Limit
**No Go source file (`.go`) may exceed 500 physical lines of code.** This limit includes all blank lines, comments, imports, and brackets. If a file is reaching or exceeding this limit, you must refactor and split its logic into smaller, focused modules or domain sub-packages.

### 2. Dependency Injection (DI)
Do not use global state, package-level variables for clients, or hardcode dependencies. All services, database clients, and configuration parameters must be supplied explicitly via constructors:
```go
// Correct: Explicit constructor dependencies
func NewOrchestrator(repo domain.StateRepository, client domain.LLMClient) *Orchestrator {
    return &Orchestrator{repo: repo, client: client}
}
```

### 3. Context Propagation
All functions performing I/O, database writes, git operations, or LLM network requests must accept `context.Context` as their first parameter. Do not store contexts inside structs.

---

## Extending the Tool Registry

The stateless agent interacts with the workspace by executing actions. To add a new capability, implement the `Tool` interface:

```go
package services

import (
	"context"
	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

type MyCustomTool struct{}

func (t *MyCustomTool) Name() string {
	return "my_custom_tool"
}

func (t *MyCustomTool) Execute(ctx context.Context, state *domain.State, args map[string]interface{}) (string, error) {
	// 1. Parse and validate arguments
	// 2. Perform safe operations
	// 3. Update the state if needed
	// 4. Return action log response
	return "Successfully executed custom tool", nil
}
```

After implementing the tool, register it in the initialization pipelines inside `cmd/noctifab/cli/start_runner.go`:

```go
reg.Register(&services.MyCustomTool{})
```

---

## Defining Permission Profiles

Security and safety are enforced via authorization profiles defined under the `profiles:` section of `.noctifab/config.yaml`. When an agent executes an action, the `PolicyValidator` checks the active profile for that role:

```yaml
profiles:
  generator:
    allowed_tools:
      - "read_file"
      - "write_file"
      - "write_files"
      - "edit_file"
      - "apply_patch"
      - "run_tests"
      - "run_linter"
      - "noop"
```

- Wildcard `*` permissions should only be granted to the `orchestrator` role profile.
- Restricting network and write tools on default profiles prevents model hallucinations from executing unsafe operations on the host.

---

## Testing Workflows

### 1. Running Unit Tests
Unit tests are co-located in the packages they test and must run clean:
```bash
go test -v ./...
```

### 2. Test-Driven Development (TDD) Specifications
Tests are written by the Tester Agent before implementation. The Test Validator executes the workspace's tests:
- **E2E tests** for happy paths.
- **Unit tests** for input validation and simple edge cases.
- **Integration tests** for complex internal validation flows and multi-component interactions.

### 3. E2E Integration Testing
The E2E test suite validates the orchestration loop end-to-end under mock scenarios:
- **Mock LLM**: A mock server simulating provider completions and custom tool actions.
- **Mock VCS**: A CGI-based Git server simulating GitHub API and repository merges.
- Run tests via the project `Makefile`:
  ```bash
  make test-e2e
  ```

### 4. Running Validation Projects (Local E2E Matrix)
To validate the system implementing features autonomously within isolated target directories, run a validation project container (e.g. `ninline`, `wc`, `pyedis`, `t4`):
```bash
make validate PROJECT=ninline
```
To run all validation projects in parallel:
```bash
make validate-all
```
For the recommended execution order, capability ladder, and tier classification, consult [`validation/projects/TESTING_GUIDE.md`](../validation/projects/TESTING_GUIDE.md).
*Note: These E2E validation runs utilize host compiler and package manager mount caching (Go and Cargo) to speed up iterations and support near-instantaneous incremental testing.*

---

## Formatting and Linting

Before pushing code or submitting pull requests, ensure that:
1. `go fmt ./...` runs clean.
2. The project's static analysis linter passes without violations. You can run the linter in the standard Docker container wrapper:
   ```bash
   docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run
   ```

---

## Adding a New LLM Provider

All LLM provider infrastructure lives in `pkg/infrastructure/llm/`. The package uses a **Provider Registry** architecture with **Go struct embedding** for composition:

- `provider_registry.go` — defines `ProviderSpec`, `RegisterProvider`, `GetProviderSpec`, and the `NewModelParser` declarative composition engine.
- `openai.go` — defines `baseOpenAIClient`, the reusable OpenAI wire-protocol base that all OpenAI-compatible providers embed.
- One dedicated `.go` file per provider (e.g. `mistral.go`, `moonshot.go`).

There are two patterns depending on whether your provider speaks the **OpenAI-compatible API** or a **custom API**.

---

### Pattern A — OpenAI-Compatible Provider

Use this pattern when the provider exposes a `/v1/chat/completions` and `/v1/models` API compatible with the OpenAI protocol (e.g. Mistral, DeepSeek, Groq, Together AI, xAI Grok).

**Create `pkg/infrastructure/llm/myprovider.go`:**

```go
package llm

import "time"

// MyProviderClient wraps baseOpenAIClient to inherit the full OpenAI
// wire protocol (Call, GetAvailableModels, etc.) via Go struct embedding.
type MyProviderClient struct {
    *baseOpenAIClient
}

// NewMyProviderClient creates an OpenAI-compatible client for MyProvider.
func NewMyProviderClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
    return &MyProviderClient{
        baseOpenAIClient: newBaseOpenAIClient(
            "myprovider",
            "https://api.myprovider.com/v1", // base URL
            url,
            timeout,
            idleTimeout,
            streaming,
        ),
    }
}

func init() {
    RegisterProvider(&ProviderSpec{
        Name:           "myprovider",
        BaseURL:        "https://api.myprovider.com/v1",
        EnvKeys:        []string{"MYPROVIDER_API_KEY"},   // checked in order; first non-empty wins
        ParseModelFunc: parseMyProviderModel,
        Protocol:       "openai",
        NewClientFunc:  NewMyProviderClient,
    })
}

// parseMyProviderModel ranks models by capacity tier.
// Adapt the tiers and keywords to the actual model names your provider uses.
var parseMyProviderModel = NewModelParser(ParserConfig{
    RequiredPrefix: "myprovider",         // only parse models whose name contains this prefix (optional)
    DefaultVersion: 1.0,
    VersionRegexp:  `([0-9]+\.[0-9]+)`,   // extract version number from model name (optional)
    Tiers: []KeywordTier{
        {Keywords: []string{"ultra", "max"}, Score: 50, TierName: "ultra"},
        {Keywords: []string{"pro", "large"}, Score: 30, TierName: "pro"},
        {Keywords: []string{"mini", "lite"}, Score: 10, TierName: "lite"},
    },
    // Use StandardSizeWeights if the provider names models by parameter count (e.g. 70b, 8b):
    // SizeWeights: StandardSizeWeights,
})
```

**That's it.** No other files need to be modified. The `init()` function registers the provider into the global registry. `client.go` will automatically use `spec.NewClientFunc` to instantiate the client and `spec.EnvKeys` to resolve the API key.

**Add the API key** to `docs/secrets.md` and `docs/configuration.md`.

---

### Pattern B — Custom API Provider

Use this pattern when the provider uses a **different wire protocol** from OpenAI (e.g. Anthropic uses `X-Api-Key` headers and its own request/response envelope; Gemini uses query-param keys and a different JSON schema).

**Step 1 — Implement `ProviderClient`**

The interface defined in `provider.go` is:
```go
type ProviderClient interface {
    Call(ctx context.Context, model, apiKey, prompt string) ([]byte, error)
    GetAvailableModels(ctx context.Context, apiKey string) ([]string, error)
}
```

**Create `pkg/infrastructure/llm/myprovider.go`:**

```go
package llm

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "time"
)

type myProviderClient struct {
    url         string
    timeout     time.Duration
    idleTimeout time.Duration
    streaming   bool
}

func NewMyProviderClient(url string, timeout, idleTimeout time.Duration, streaming bool) ProviderClient {
    return &myProviderClient{url: url, timeout: timeout, idleTimeout: idleTimeout, streaming: streaming}
}

func (c *myProviderClient) Call(ctx context.Context, model, apiKey, prompt string) ([]byte, error) {
    endpoint := "https://api.myprovider.com/v1/generate"
    if c.url != "" {
        endpoint = c.url
    }

    payload, _ := json.Marshal(map[string]any{
        "model":  model,
        "prompt": prompt,
    })

    req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(payload))
    if err != nil {
        return nil, err
    }
    req.Header.Set("Authorization", "Bearer "+apiKey)
    req.Header.Set("Content-Type", "application/json")

    timeout := c.timeout
    if timeout <= 0 {
        timeout = 10 * time.Minute
    }
    httpClient := &http.Client{Timeout: timeout}
    resp, err := httpClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, err
    }
    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(body))
    }

    // Parse the provider-specific response envelope and return the text content.
    var result struct {
        Output string `json:"output"`
    }
    if err := json.Unmarshal(body, &result); err != nil {
        return nil, err
    }
    return []byte(result.Output), nil
}

func (c *myProviderClient) GetAvailableModels(ctx context.Context, apiKey string) ([]string, error) {
    // Query the provider's model list endpoint. Return model name strings.
    // If the provider has no models endpoint, return a hardcoded sentinel — but
    // NOTE: do NOT return a hardcoded static list of model names; always query live.
    return nil, fmt.Errorf("myprovider: model listing not supported")
}

func init() {
    RegisterProvider(&ProviderSpec{
        Name:          "myprovider",
        BaseURL:       "https://api.myprovider.com/v1",
        EnvKeys:       []string{"MYPROVIDER_API_KEY"},
        Protocol:      "myprovider", // any unique string; NOT "openai"
        NewClientFunc: NewMyProviderClient,
        ParseModelFunc: NewModelParser(ParserConfig{
            DefaultVersion: 1.0,
            Tiers: []KeywordTier{
                {Keywords: []string{"large"}, Score: 30, TierName: "large"},
                {Keywords: []string{"small"}, Score: 10, TierName: "small"},
            },
        }),
    })
}
```

**Step 2 — Write unit tests** in `myprovider_test.go` covering `Call` (use `httptest.NewServer` to mock the endpoint), `GetAvailableModels`, and the `ParseModelFunc`.

**Step 3 — Add credentials** to `docs/secrets.md` and `docs/configuration.md`.

---

### Model Capacity Ranking (`ParseModelFunc`)

The `ParseModelFunc` is used by the **Dynamic Model Fallback Engine** to rank models by capacity so it can automatically retry with a lower-capability model when the primary model fails. Always define it.

`NewModelParser(ParserConfig{...})` accepts the following fields:

| Field | Purpose | Example |
|---|---|---|
| `RequiredPrefix` | Only parse models whose lowercased name contains this string | `"claude"`, `"grok"` |
| `DefaultVersion` | Fallback version if `VersionRegexp` doesn't match | `3.0` |
| `VersionRegexp` | Regex to extract a version number from the model name | `` `claude-([0-9]+(?:\.[0-9]+)?)` `` |
| `VersionMultiplier` | Multiplier applied to extracted version in rank calculation | `5` (default: `10`) |
| `Tiers` | Ordered list of keyword→score mappings for named model tiers | `{Keywords: []string{"opus"}, Score: 400, TierName: "opus"}` |
| `SizeWeights` | Map of parameter size suffixes to scores for open-weights models | `StandardSizeWeights` (405b→500, 70b→400, 8b→200…) |
| `ContextBonus` | Add a small bonus rank for long-context variants (128k, 32k, 8k) | `true` |

Final rank = `baseScore + version * VersionMultiplier + contextBonus`.
Higher rank = higher capacity = tried first; lower rank = fallback.
