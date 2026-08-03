# Specification: Fortune Quote Generator in C

This document defines the specification for `fortune`, a high-performance C command-line application that displays a random quote/sentence from a fixed SQLite database containing 100 famous quotes.

## 1. Project Layout & DDD Architecture

To maintain high cohesion, testability, and strict SOLID principles in C, the codebase must adhere to Domain-Driven Design (DDD) packaging boundaries:

- `include/domain/quote.h` - Domain model representing the `Quote` entity (ID, author string, sentence string) and domain validation logic.
- `include/domain/quote_repository.h` - Abstract repository interface definition for quote retrieval.
- `src/domain/quote.c` - Domain entity logic implementation.
- `include/infrastructure/sqlite_quote_repository.h` - SQLite database adapter interface.
- `src/infrastructure/sqlite_quote_repository.c` - Implementation of SQLite persistence layer using official SQLite C interface (`sqlite3.h`).
- `include/application/fortune_service.h` - Application service layer interface.
- `src/application/fortune_service.c` - Application service implementation fetching and formatting random quotes.
- `main.c` - CLI application entrypoint.
- `Makefile` - Build, test, format, and lint automation.
- `tests/test_quote.c` - Unit tests for quote domain model.
- `tests/test_sqlite_repository.c` - Unit tests for SQLite persistence repository.
- `tests/test_fortune_service.c` - Unit tests for application service logic.

## 2. Technical & Compliance Requirements

### 2.1 C Language Standard & Compiler
1. **Language Standard:** Code MUST conform strictly to the latest ISO C ANSI standard (C17 / ISO/IEC 9899:2018).
2. **Compiler:** Compilation MUST be executed with GCC (`gcc`) using strict warnings and pedantic rules:
   ```bash
   gcc -Wall -Wextra -Werror -pedantic -std=c17
   ```
3. **Best Practices:** No non-standard compiler extensions (e.g. GNU C extensions). Pure standard ANSI C.

### 2.2 Memory Efficiency & Low-Allocation Guarantee
1. **Minimal Heap Memory:** The application must reserve memory as sparingly as possible. Prefer stack allocation for fixed-size string buffers where safe.
2. **Zero Memory Leaks:** Every `malloc` / `calloc` / `strdup` MUST be paired with an explicit `free`. SQLite statements (`sqlite3_stmt*`) and database connections (`sqlite3*`) MUST be finalized with `sqlite3_finalize` and `sqlite3_close`.
3. **Buffer Safety:** Use bounds-checked formatting functions (`snprintf`, `strncpy`) to prevent buffer overflows.

### 2.3 SQLite C Connection Interface
1. Connection layer MUST use official SQLite C API functions ([https://sqlite.org/c_interface.html](https://sqlite.org/c_interface.html)):
   - `sqlite3_open_v2` / `sqlite3_open`
   - `sqlite3_prepare_v2`
   - `sqlite3_step`
   - `sqlite3_column_text`
   - `sqlite3_column_int`
   - `sqlite3_finalize`
   - `sqlite3_close`
2. **Database Database Schema:** Table `quotes` with columns:
   - `id` INTEGER PRIMARY KEY AUTOINCREMENT
   - `author` TEXT NOT NULL
   - `sentence` TEXT NOT NULL
3. **Database Population:** The database MUST be seeded with the 100 famous quotes defined in [seed.sql](seed.sql). The application or build system MUST execute `seed.sql` or read `seed.sql` to populate `fortune.db`.

### 2.4 CLI Output & Features
1. **Random Quote Selection:** When executed (`./fortune` or `make run`), the program randomly selects 1 quote from the 100 famous sentences in SQLite and prints it to stdout in standard format:
   ```
   "The only true wisdom is in knowing you know nothing."
     -- Socrates
   ```
2. **Exit Code:** Return code `0` on successful execution; return code `1` on database error.

## 3. Build & Testing Automation (Makefile)

The project root MUST include a `Makefile` supporting the following targets:
- `all`: Builds the main binary executable `./fortune`.
- `build`: Compiles C source files into object files and links binary.
- `test`: Builds and runs all C unit test suites. Exits 0 on all tests passing.
- `lint`: Runs GCC static syntax and constraint check (`gcc -Wall -Wextra -Werror -pedantic -std=c17 -fsyntax-only`).
- `format`: Runs code formatting (e.g. `gcc -Wall -Wextra -Werror -pedantic -std=c17 -fsyntax-only` or a clang-format pass). MUST always exist because the build automation invokes `make format`.
- `clean`: Removes object files, binaries, and build artifacts.

IMPORTANT: Each target MUST be defined EXACTLY ONCE in the Makefile. Do not define
`test`, `lint`, or `format` more than once (duplicate recipes override each other and
warn), and never omit `lint` or `format`.

## 4. Verification Requirements

1. **Unit Test Coverage:** 100% test coverage for domain models, SQLite repository query logic, and application service formatting.
2. **Execution Command:**
   ```bash
   make test
   ```

---

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
        "content": "# User Story 001: Implement Fortune Quote Generator in C\n\nAs a user, I want a C command-line application that retrieves and displays a random famous quote from an SQLite database containing 100 quotes.\n\n## Requirements\n- Write standard C code conforming to C17 standard compiled with GCC (gcc -Wall -Wextra -Werror -pedantic -std=c17).\n- Follow SOLID principles and DDD layer structure: include/domain/, src/domain/, include/infrastructure/, src/infrastructure/, include/application/, src/application/, main.c, Makefile.\n- Use official SQLite C API interface (sqlite3_open, sqlite3_prepare_v2, sqlite3_step, sqlite3_column_text, sqlite3_finalize, sqlite3_close).\n- Seed SQLite database fortune.db using the provided seed.sql file containing 100 famous quotes.\n- Memory optimization: minimal heap allocations, zero memory leaks, explicit free.\n- Build system Makefile with targets: all, build, test, lint, format, clean. Each target MUST be defined exactly once (no duplicated recipes). lint and format MUST NOT be omitted.\n- 100% unit test coverage under tests/ running via make test.\n\n## Validation Criteria\n- Unit tests pass with make test.\n- Program executes and displays random quote.\n\n---\ndepends_on: []\nchange_type: new"
      }
    }
  ]
}
