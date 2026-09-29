# WC Project Specification

## 1. Overview
The goal of this project is to implement a command-line interface (CLI) application in Rust that replicates the core functionality of the standard UNIX `wc` (word count) utility. The program must count lines, words, characters, bytes, and maximum line length from input files or standard input.

To ensure high maintainability, safety, and suitability for E2E validation, the codebase must adhere strictly to SOLID principles, Domain-Driven Design (DDD) patterns, and memory-efficient streaming.

### 1.1. Toolchain & Project Metadata
- Rust edition 2021; MSRV 1.74.
- Crate name: `wc` (binary name `wc`). Integration tests MUST invoke the built binary via `assert_cmd::Command::cargo_bin("wc")`, never the system `wc`.
- `Cargo.toml` is a workspace-root single crate (no workspaces).
- Allowed external crates: `clap` (v4, derive) for CLI parsing; `assert_cmd`, `predicates`, `tempfile` for tests only. No other dependencies.
- Lint: `cargo fmt --check` and `cargo clippy -- -D warnings` must pass.
- Testing & Concurrency Invariant: Tests are plain Rust unit and integration tests under `tests/`. All tests must execute deterministically under standard `cargo test` with thread-safe test isolation (using `tempfile` and isolated stream buffers) without acquiring external daemon locks or requiring concurrent compilation processes.
- NOTE: This SPEC scopes only the `wc` Rust target. The repository-root AGENTS.md applies to the noctifab Go host and its BDD rules do NOT transfer here. Tests are plain Rust `#[test]` tests under `tests/`.

---

## 2. Architecture & Code Constraints

### 2.1. Domain-Driven Design (DDD) & SOLID Layout
The Rust project must be structured into separate layers:
1.  **Domain Layer:** Pure Rust (no external system dependencies). Defines core domain models like `CountStats` (representing counts of lines, words, chars, bytes, max-line-length), and the traits/interfaces for counting (e.g. `CountStrategy`).
2.  **Application Layer:** Contains orchestrating use cases, such as `CountUseCase` that coordinates reading from multiple sources, calculating stats, and accumulating totals. It depends on traits, utilizing Dependency Injection (DI).
3.  **Infrastructure Layer:** Implements concrete adapters, such as command-line argument parsing, file reading adapters, stdout printers, and stdin access.

Pinned directory layout (every file path in user stories MUST be interpreted against this tree):
```
src/
  lib.rs               // declares #![deny(unsafe_code)]
  main.rs              // thin entry; delegates to application
  domain/
    count_stats.rs     // struct CountStats { lines, words, chars, bytes, max_line_len }
    count_strategy.rs  // trait CountStrategy { fn count(&mut self, &mut dyn Read); fn result(self) -> CountStats; }
  application/
    count_usecase.rs   // struct CountUseCase<R: Read>; orchestrates
    aggregate.rs       // total aggregation (Sum / Max)
  infrastructure/
    cli.rs             // clap Parser
    readers.rs         // file/stdin adapters
    output.rs          // formatter + printer
tests/
  integration_*.rs     // integration tests (assert_cmd + tempfile)
```

**SOLID Rules:**
*   **SRP:** Separate parsing, input reading, core counting algorithms, and output formatting.
*   **OCP:** Introduce a trait for counting strategies so new metrics can be added without modifying the core orchestrator.
*   **DIP:** The application orchestrator must receive standard readers/inputs via traits or standard generic types (e.g. `std::io::Read`) rather than referencing hardcoded file reading.

### 2.2. Safety Constraints
*   **No Unsafe Code:** The use of `unsafe` Rust blocks is strictly prohibited. The main/library entry point must declare `#![deny(unsafe_code)]` to enforce this rule at compile-time.

### 2.3. Memory & Large File Constraints
*   **Multi-Gigabyte Support:** The program must handle files of several gigabytes (e.g., 10GB+) under a constant, low memory footprint.
*   **Streaming I/O:** Do not load full file contents into memory. Implement streaming using buffered reading (e.g., `std::io::BufReader`) with fixed-size chunks (e.g. 64KB or 128KB). Memory consumption must be $O(1)$ relative to the size of the input file.

---

## 3. Requirements & Option Behavior

### 3.1. CLI Options
The compiled binary should support the following options:
*   `-l` / `--lines`: Count the number of newlines (`'\n'`).
*   `-w` / `--words`: Count the number of words (sequences of characters delimited by whitespace).
*   `-c` / `--bytes`: Count the number of bytes.
*   `-m` / `--chars`: Count the number of valid UTF-8 characters.
*   `-L` / `--max-line-length`: Count the UTF-8 character length of the longest line (excluding the trailing `'\n'`).
*   Short flags may be bundled (e.g. `-lw` == `-l -w`). Long flags do not accept `=`-syntax. `--` terminates option parsing.

### 3.2. Default Behavior
*   If no options are specified, default to `-l`, `-w`, and `-c` (newline, word, and byte counts).
*   If multiple files are provided, process them sequentially, print the counts for each, and output a final `total` row with accumulated counts (for `-L`, the `total` is the maximum line length found among all files).
*   Do NOT print a `total` row when only one input source (file or stdin) was processed. The `total` row is printed only when ≥2 input sources are processed.

### 3.3. Output Format
*   Counts are printed in the canonical order **lines, words, chars, bytes, max-line-length**, filtered to the active flags, **independent of the order in which flags were passed on the command line**.
*   Each count column is formatted with Rust `format!("{:>7}", n)` (7-wide, right-aligned, space-padded). Columns are separated by a single space.
*   If a filename is shown, it is preceded by a single space and printed verbatim as given on the command line.
*   If reading from stdin or `-`, no filename is printed (and no trailing space).
*   Reference formatter: `format!("{:>7} {:>7} {:>7} {}", lines, words, bytes, name)` (omit unused columns and the `name` segment as appropriate).

### 3.4. Counting Semantics
*   "Whitespace" for `-w` means Rust `char::is_whitespace` (Unicode). A "word" is a maximal run of non-whitespace characters.
*   `-c` counts all bytes read, including a trailing newline.
*   `-m` counts Unicode codepoints decoded from valid UTF-8. Invalid byte sequences are counted one codepoint per byte and continue (do not abort, do not error). Matches GNU `wc` `mbrtowc` fallback.
*   `-L` is the length, in characters (Unicode codepoints), of the longest line, excluding the terminating `'\n'`. A trailing `'\r'` (as in `\r\n`) IS counted toward the line length. A final line without a newline IS considered toward the maximum. Empty input → max = 0.
*   `-l` counts `'\n'` bytes; a final line without `'\n'` is not counted.

### 3.5. Errors & Exit Codes
*   Missing/unreadable file: write to stderr exactly `wc: <name>: <os error display>\n`, continue processing remaining inputs, omit the failed input from stdout and from `total`, exit code **1**.
*   Invalid CLI option: write to stderr exactly `wc: invalid option -- '<char>'\nTry 'wc --help' for more information.\n` and exit immediately with code **2**.
*   Mid-stream I/O error: same template as missing file, exit 1.
*   All error messages use the literal program name `wc` (not `argv[0]`).
*   Directory input is out of scope for v1: treat as a missing/unreadable file using the OS error returned by `File::open` (e.g. "Is a directory").

---

## 4. Execution Examples

### 4.1. Single File Default Behavior (Lines, Words, Bytes)
Command:
```bash
wc input.txt
```
Output:
```
      12      150     1024 input.txt
```

### 4.2. Stdin Default Behavior
Command:
```bash
cat input.txt | wc
```
Output:
```
      12      150     1024
```

### 4.3. Specific Option Combinations
Command:
```bash
wc -l -c input.txt
```
Output:
```
      12     1024 input.txt
```

### 4.4. Long Options
Command:
```bash
wc --lines --max-line-length input.txt
```
Output:
```
      12       80 input.txt
```

### 4.5. UTF-8 Character Counting
Given `unicode.txt` whose exact contents are the UTF-8 bytes of `Hello, 世界!` followed by a single trailing `\n`. Breakdown: `Hello, ` = 7 chars / 7 bytes; `世` = 1 char / 3 bytes; `界` = 1 char / 3 bytes; `!` = 1 char / 1 byte; `\n` = 1 char / 1 byte. Totals: **11 characters, 15 bytes**.
Command:
```bash
wc -c -m unicode.txt
```
Output (canonical order: chars then bytes):
```
      11       15 unicode.txt
```

### 4.6. Multi-file Processing
Command:
```bash
wc -l -w file1.txt file2.txt
```
Output:
```
      10       45 file1.txt
       5       20 file2.txt
      15       65 total
```

### 4.7. Mixing Stdin and Files
Command:
```bash
cat file1.txt | wc -l -w - file2.txt
```
Output:
```
      10       45 -
       5       20 file2.txt
      15       65 total
```

### 4.8. Missing File Error Handling
Command:
```bash
wc -l file1.txt missing.txt file2.txt
```
Stderr:
```
wc: missing.txt: No such file or directory
```
Stdout:
```
      10 file1.txt
       5 file2.txt
      15 total
```
Exit code: `1`

---

## 5. Verification Criteria & Testing

### 5.1. Unit Testing (Mocking allowed)
*   **Isolation:** Test the domain and application layers in isolation.
*   **Mocking:** Use mock readers/inputs (e.g. testing parsing and counting logic using mock traits or custom stub implementations of standard Rust readers).
*   **UTF-8 and Edge Cases:** Test empty files, binary files, files with no newlines, and multi-byte UTF-8 sequences.

### 5.2. Integration Testing (No Mocking)
*   **End-to-End CLI Checks:** Execute the compiled CLI binary as a subprocess against actual files on disk and standard input.
*   **Large File Test:** Verify execution on large input streams (e.g. piping 1GB+ generated streams) to confirm memory usage remains constant ($O(1)$) and does not leak or crash.
*   **Exit Status:** Check that errors (missing files or invalid flags) return non-zero exit codes.


## 5. Definition of Done (DoD)

To consider `wc` fully implemented, the Rust CLI tool must satisfy:
1. **Public Binary:** `wc-cli` binary correctly counts bytes (`-c`), lines (`-l`), words (`-w`), and characters (`-m`) from stdin or files, matching standard POSIX `wc` behavior and exiting code `0`.
2. **Linting Invariant:** Zero warnings under `cargo fmt --check` and `cargo clippy -- -D warnings`.
3. **Verification Criteria:** 100% test pass rate executing `cargo test`.
