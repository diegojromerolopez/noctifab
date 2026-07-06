# Specification: Ruby Terminal Calculator

This document defines the specification for a simple terminal-based calculator written in Ruby.

## 1. Project Layout

To keep the codebase modular, testable, and clean, the files must follow a Domain-Driven Design (DDD) layout:

- `calculator.rb` - The CLI application main entry point.
- `lib/calculator/engine.rb` - Core domain model containing mathematical operations.
- `lib/calculator/cli.rb` - Interface layer for the CLI argument parsing and REPL loop.
- `spec/spec_helper.rb` - RSpec testing configuration.
- `spec/unit/engine_spec.rb` - Unit tests for the core engine logic.
- `spec/integration/cli_spec.rb` - Integration tests verifying command line interactions and REPL behavior.

## 2. Core Features

### 2.1 Mathematical Operations

The calculator engine must support the following operations on float or integer numbers:
1. **Addition (`+` or `add`):** Returns sum of two numbers.
2. **Subtraction (`-` or `sub`):** Returns difference of two numbers.
3. **Multiplication (`*` or `mul`):** Returns product of two numbers.
4. **Division (`/` or `div`):** Returns quotient of two numbers. If the divisor is 0 or 0.0, the engine MUST raise ZeroDivisionError.
5. **Power (`^` or `pow`):** Returns the base raised to the exponent (e.g., `2 ^ 3 = 8.0`).
6. **Modulo (`%` or `mod`):** Returns remainder of division.
7. **Square Root (`sqrt`):** Returns the square root of a single number (unary operation).

### 2.2 Execution Modes

#### CLI Argument Mode
If the script is executed with arguments (e.g., `ruby calculator.rb 2 + 3`), it must parse the arguments, evaluate the expression, write the result to stdout, and exit with code 0.
Examples:
- `ruby calculator.rb 12 * 3` -> Prints `36.0`
- `ruby calculator.rb 2 ^ 8` -> Prints `256.0`
- `ruby calculator.rb 16 sqrt` -> Prints `4.0`

#### Interactive REPL Mode
If the script is executed with no arguments (e.g., `ruby calculator.rb`), it must launch an interactive prompt loop:
1. Print the prompt `calc> ` to stdout (without a trailing newline).
2. Read a line of input from stdin.
3. If the input is `exit` or `quit`, or if stdin reaches EOF (Ctrl+D), exit the loop with code 0.
4. Parse and evaluate the input expression.
5. If the expression is evaluated successfully, print the result to stdout.
6. Loop back to step 1.

### 2.3 Error Handling & Constraints

All error messages must be printed to `stderr`.

1. **Division by Zero:**
   - Operation: Division where the divisor is `0` or `0.0`.
   - Behavior: Print `Error: Division by zero` to stderr.
   - CLI Mode Exit Code: `1`
   - REPL Mode: Print error to stderr and prompt again.

2. **Negative Square Root:**
   - Operation: Unary `sqrt` on a negative number.
   - Behavior: Print `Error: Square root of a negative number` to stderr.
   - CLI Mode Exit Code: `1`
   - REPL Mode: Print error to stderr and prompt again.

3. **Invalid Expression / Parse Error:**
   - Operation: Any malformed or unsupported input.
   - Behavior: Print `Error: Invalid expression` to stderr.
   - CLI Mode Exit Code: `1`
   - REPL Mode: Print error to stderr and prompt again.

## 3. Formatting & Linting

1. **Style Guidelines:** All code must pass RuboCop linting without warnings or errors. The code generation process and any modifications must respect and use the `.rubocop.yml` style configuration located in the root folder of the project. Every time a feature is complete, the agent must run the linter process, and format the code until the linter passes. Run the linter command:
   ```bash
   rubocop
   ```
2. **Standard Format:** Use 2 spaces for indentation, clean Ruby style, and define helper classes inside the `Calculator` namespace.

## 4. Verification Requirements

1. **Unit Tests:** Verify all math operations, edge cases, and error cases in isolation using RSpec. CRITICAL: Keep unit test files simple, concise, and under 100 lines of code to prevent token truncation.
2. **Integration Tests:** Verify execution modes (CLI arguments and REPL prompting) by capturing process stdout/stderr and verifying output formats. Keep integration test files simple, concise, and under 100 lines.
3. **Execution Command:** All tests must pass cleanly:
   ```bash
   rspec spec/
   ```


## 5. Product Manager Instructions

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
        "content": "# User Story 001: Implement Ruby Terminal Calculator\n\nAs a developer, I want to implement the complete Ruby terminal calculator so that users can perform mathematical operations in both CLI and interactive REPL modes.\n\n## Requirements\n- The project is already pre-configured with Gemfile, .rubocop.yml, Rakefile, and spec/spec_helper.rb. Do NOT plan any tasks to set up, initialize, test, or modify these pre-configured files. Do NOT create a bin/setup script. Do NOT write any tests checking if files are executable, as file permissions cannot be modified in this sandbox.\n- CRITICAL: To prevent token truncation, all unit and integration test files written by the Tester Agent (e.g. spec/calculator/engine_spec.rb) MUST be kept simple, concise, and under 100 lines. Do NOT write verbose or repetitive tests.\n- CRITICAL: The Tester Agent is only authorized to write and edit files inside the spec/ directory. The Tester Agent must never write or edit files inside lib/ or the root directory.\n- CRITICAL: The Generator Agent is only authorized to write and edit implementation files (e.g. calculator.rb and lib/ calculator/ files). The Generator Agent must never modify files inside the spec/ directory.\n- Implement calculator.rb as the main entry point.\n- Implement lib/calculator/engine.rb containing mathematical operations: +, -, *, /, ^, %, sqrt. Do NOT create lib/calculator/core.rb.\n- Division by zero in lib/calculator/engine.rb must raise ZeroDivisionError if the divisor is 0 or 0.0.\n- Implement lib/calculator/cli.rb for argument parsing and interactive REPL mode.\n- Error handling: division by zero, negative square root, and invalid expression. Print errors to stderr and return correct exit codes.\n- Indentation: 2 spaces. Code must pass rubocop.\n\n## Validation Criteria\n- Unit tests in spec/unit/engine_spec.rb.\n- Integration tests in spec/integration/cli_spec.rb.\n- Run tests with rspec spec/.\n\n\n---\ndepends_on: []\nchange_type: new"
      }
    }
  ]
}



