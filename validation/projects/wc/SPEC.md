# WC Project Specification

## 1. Overview
The goal of this project is to implement a command-line interface (CLI) application in Rust that replicates the core functionality of the standard UNIX `wc` (word count) utility. The program must count lines, words, characters, bytes, and maximum line length from input files or standard input.

To ensure high maintainability, safety, and suitability for E2E validation, the codebase must adhere strictly to SOLID principles, Domain-Driven Design (DDD) patterns, and memory-efficient streaming.

---

## 2. Architecture & Code Constraints

### 2.1. Domain-Driven Design (DDD) & SOLID Layout
The Rust project must be structured into separate layers:
1.  **Domain Layer:** Pure Rust (no external system dependencies). Defines core domain models like `CountStats` (representing counts of lines, words, chars, bytes, max-line-length), and the traits/interfaces for counting (e.g. `CountStrategy`).
2.  **Application Layer:** Contains orchestrating use cases, such as `CountUseCase` that coordinates reading from multiple sources, calculating stats, and accumulating totals. It depends on traits, utilizing Dependency Injection (DI).
3.  **Infrastructure Layer:** Implements concrete adapters, such as command-line argument parsing, file reading adapters, stdout printers, and stdin access.

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

### 3.2. Default Behavior
*   If no options are specified, default to `-l`, `-w`, and `-c` (newline, word, and byte counts).
*   If multiple files are provided, process them sequentially, print the counts for each, and output a final `total` row with accumulated counts (for `-L`, the `total` is the maximum line length found among all files).

### 3.3. Output Format
*   Counts must be printed in the order: lines, words, chars, bytes, max-line-length (depending on which flags are active).
*   To match standard UNIX `wc`, counts should be formatted right-aligned in columns of width 8.
*   If a filename is provided, it is appended to the counts, separated by a space.
*   If reading from stdin or `-`, no filename is printed.

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
Given `unicode.txt` containing `Hello, 世界!` (13 characters, 15 bytes):
Command:
```bash
wc -c -m unicode.txt
```
Output:
```
      13       15 unicode.txt
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
