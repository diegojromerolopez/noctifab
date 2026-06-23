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
package usecase

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

After implementing the tool, register it in the initialization pipelines inside `cmd/noctifab/cli/start.go` and `cmd/noctifab/cli/run_once.go`:

```go
reg.Register(&usecase.MyCustomTool{})
```

---

## Defining Permission Profiles

Security and safety are enforced via authorization profiles under `.noctifab/profiles/`. When an agent executes an action, the `PolicyValidator` checks the active profile (e.g., `generator.yaml`, `default.yaml`):

```yaml
permissions:
  allowed_tools:
    - "read_file"
    - "run_tests"
    - "noop"
  network:
    allow_ai_provider: true
    allow_external: false
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

### 2. BDD Holdout Specifications
Acceptance scenarios gate task quality. These are written in a Behavior-Driven Development (BDD) context pattern:
```go
// Example in test execution
When("submitting task completion", func() {
    It("must pass the holdout test suite", func() {
        // Assertions
    })
})
```

### 3. E2E Integration Testing
The E2E test suite validates the orchestration loop end-to-end under mock scenarios:
- **Mock LLM**: A mock server simulating provider completions and custom tool actions.
- **Mock VCS**: A CGI-based Git server simulating GitHub API and repository merges.
- Run tests via the project `Makefile`:
  ```bash
  make test-e2e
  ```

---

## Formatting and Linting

Before pushing code or submitting pull requests, ensure that:
1. `go fmt ./...` runs clean.
2. The project's static analysis linter passes without violations. You can run the linter in the standard Docker container wrapper:
   ```bash
   docker run -t --rm -v $(pwd):/app -w /app golangci/golangci-lint:v2.12.2 golangci-lint run
   ```
