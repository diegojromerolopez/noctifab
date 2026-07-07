# Echo CLI Specification

This document defines the specification for a minimal command-line utility called `echo` written in Go.

## 1. Overview
The goal of this project is to implement a command-line interface (CLI) application that prints in the standard output the first argument it receives (as if it were the `echo` function).

---

## 2. Requirements

### 2.1. CLI Invocation
The compiled binary (`echo-cli`) should support execution with variable command-line arguments:

1. **With Arguments**
   * **Command:** `echo-cli <arg1> <arg2> ...`
   * **Behavior:** Prints the first argument (`arg1`) followed by a newline `\n` to standard output, and exits with code 0.
   * **Example:** `echo-cli hello world` prints `hello` and a newline.

2. **Without Arguments**
   * **Command:** `echo-cli`
   * **Behavior:** Prints a single newline `\n` to standard output, and exits with code 0.

### 2.2. Architecture
To keep the codebase modular, testable, and clean, the files must follow a simple layout:

- `go.mod` - Go module definition (module path `github.com/noctifab/echo`, Go 1.22+).
- `cmd/echo/main.go` - The entry point (composition root), which parses CLI inputs, calls the core business logic, prints the formatted result, and handles exit codes.
- `pkg/echoer/echoer.go` - Core domain/business logic package containing the helper function `Echo(args []string) string` to handle the logic.
- `pkg/echoer/echoer_test.go` - Unit tests for the core echoer package.
- `tests/cli_integration_test.go` - Integration tests verifying command line interactions.
- `Makefile` - Project automation rules, defining `test` and `lint` targets.

No Go source code file may exceed 500 lines of code.

### 2.3. Linter & Formatting
1. **Formatting:** All Go source code must strictly follow the standard `go fmt` format.
2. **Linting:** Code must pass `go vet ./...` without warnings or errors.

---

## 3. Verification Criteria & Testing

### 3.1. Unit Tests
* Unit tests (in `pkg/echoer/echoer_test.go`) verify the behavior of `Echo` under different inputs (e.g. single argument, multiple arguments, no arguments).

### 3.2. Integration Tests
* Integration tests (in `tests/cli_integration_test.go`) compile the binary to a temporary path (building the package `github.com/noctifab/echo/cmd/echo`), invoke it with various arguments (or no arguments) using subprocess execution, and assert correct stdout, stderr (empty), and exit code (0).

### 3.3. Test Execution Command
* All tests must pass:
  ```bash
  go test -v ./...
  ```

## 4. Product Manager Instructions

You are acting as the Product Manager Agent. You must generate exactly ONE user story under `roadmap/US-001.md` that encompasses all the features described in this specification.

You MUST respond ONLY with a single JSON block containing a list of actions of tool 'create_story'. The response must not contain any markdown wrapping or other text.

JSON Schema:
{
  "reasoning": "your reasoning...",
  "actions": [
    {
      "tool": "create_story",
      "args": {
        "filename": "roadmap/US-001.md",
        "content": "# User Story 001: Implement Echo CLI\n\nAs a user,\nI want a CLI tool that prints the first argument it receives to standard output\nSo that I can see the argument echoed back.\n\n---\ndepends_on: []\nchange_type: new\ntarget_files:\n  - go.mod\n  - cmd/echo/main.go\n  - pkg/echoer/echoer.go\n  - pkg/echoer/echoer_test.go\n  - tests/cli_integration_test.go\n  - Makefile\n---\n\n## Requirements\n\n### 1. Project Scaffold & Echoer Core\n- `go.mod` (module `github.com/noctifab/echo`, Go 1.22+).\n- `pkg/echoer/echoer.go`: Implement a function `Echo(args []string) string`. It takes the command-line arguments (excluding the program name itself) and returns the first argument, or a blank string `\"\"` if there are no arguments.\n- `cmd/echo/main.go`: Composition root. Wires the command line arguments from `os.Args[1:]` to `echoer.Echo`, writes the returned string to `os.Stdout` (followed by a newline), and exits 0.\n- `Makefile` with a `test` target (`go test -v ./...`) and a `lint` target (`go vet ./...`).\n\n### 2. Echo Behavior\n- Command: `echo-cli <arguments>` (binary named `echo-cli`; tests compile it to a temporary path, e.g., `/tmp/echo-cli` or similar, and execute it).\n- If executed with arguments (e.g. `echo-cli hello world`), it prints the first argument (e.g. `hello`) followed by a newline `\\n` to `os.Stdout`, and exits with code 0.\n- If executed with no arguments (e.g. `echo-cli`), it prints a single newline `\\n` to `os.Stdout`, and exits with code 0.\n\n## Architectural & Performance Constraints\n- All file sizes must be strictly under 500 lines.\n- Wires dependencies explicitly.\n\n## Validation Criteria\n\n### Unit Tests\n- `pkg/echoer/echoer_test.go`: Unit tests for `Echo(args []string) string`. Verify that:\n  - When `args = []string{\"hello\", \"world\"}`, the result is `\"hello\"`.\n  - When `args = []string{}` (empty slice), the result is `\"\"`.\n\n### Integration Tests\n- `tests/cli_integration_test.go`: Integration tests executing the compiled binary as a subprocess.\n  - Assert that executing `echo-cli hello` prints `hello\\n` to stdout and exits with code 0.\n  - Assert that executing `echo-cli \"hello world\" foo` prints `hello world\\n` to stdout and exits with code 0.\n  - Assert that executing `echo-cli` (no arguments) prints a single `\\n` to stdout and exits with code 0."
      }
    }
  ]
}
