# `sqlasm` Technical Specification & Architecture

`sqlasm` is a high-performance, embedded B-Tree storage engine and SQL-92 relational database management system written in **pure x86_64 Intel Assembly** (`nasm`). Running inside an isolated `linux/amd64` Docker container (emulated on ARM64 macOS hosts), `sqlasm` uses native Linux 64-bit system calls (`sys_mmap`, `sys_open`, `sys_write`, `sys_fsync`) for direct page management, zero-C-runtime overhead, and deterministic execution.

---

## 1. Core Technical Invariants & Execution Rules

> [!IMPORTANT]
> **GOAL 1: PURE x86_64 ASSEMBLY & DIRECT LINUX SYSCALLS**
> - **Syntax & Assembler**: Source code MUST be written in 64-bit Intel Assembly syntax compiled with NASM (`nasm -f elf64`).
> - **System Call Interface**: All File I/O, memory allocations, and process exits MUST use native Linux `syscall` instructions (`rax` system call numbers: `sys_read=0`, `sys_write=1`, `sys_open=2`, `sys_close=3`, `sys_mmap=9`, `sys_munmap=11`, `sys_fsync=74`, `sys_exit=60`).
> - **System V AMD64 ABI**: Function calls between assembly modules MUST strictly adhere to System V AMD64 ABI (`rdi`, `rsi`, `rdx`, `rcx`, `r8`, `r9` for arguments; `rax` for return value; `rbx`, `rsp`, `rbp`, `r12`-`r15` preserved across calls).

> [!IMPORTANT]
> **GOAL 2: B-TREE PAGE STORAGE & SQL-92 ENGINE**
> - **4KB B-Tree Pages**: Data and index pages are strictly 4096 bytes. Slotted-page layout stores record offsets at page end and record bytes growing from header forward.
> - **SQL-92 Engine**: Must tokenize, parse, and execute core SQL-92 DDL (`CREATE TABLE`) and DML (`INSERT INTO`, `SELECT FROM WHERE`, `DELETE FROM`).

> [!CAUTION]
> **GOAL 3: ZERO MEMORY LEAKS & 500-LINE FILE LIMIT**
> - No single `.asm` or `.s` file may exceed **500 lines of code**.
> - Memory management must pass `valgrind --leak-check=full` with zero leaks or invalid memory accesses.
> - Compilation MUST enforce `nasm -Werror` with zero warnings.

---

## 2. Directory Layout & Assembly Modules

```
sqlasm/
├── Makefile                      # assemble, link, test, lint, e2e targets
├── Dockerfile                    # linux/amd64 container with NASM, GCC, Valgrind
├── docker-compose.yml            # Local E2E testing harness (amd64 platform on M1 Mac)
├── src/
│   ├── main.asm                  # CLI parser, sys_exit handler, top-level dispatcher
│   ├── syscalls.asm              # Syscall wrappers (sys_open, sys_mmap, sys_write, etc.)
│   ├── btree.asm                 # B-Tree page allocation, node split, binary search
│   ├── pager.asm                 # 4KB slotted page buffer manager
│   ├── lexer.asm                 # SQL-92 tokenizer (ident, string, number, keywords)
│   ├── parser.asm                # SQL-92 AST generator
│   └── vm.asm                    # Execution engine matching AST against B-Tree records
└── tests/
    ├── unit/                     # Unit tests invoking individual assembly symbols
    ├── integration/              # SQL script runner testing DDL and DML statements
    └── e2e/                      # Docker black-box SQL CLI execution script
```

---

## 3. Supported SQL-92 Subset & Binary Formats

### 3.1 Supported SQL-92 Grammar
```sql
-- Table Creation
CREATE TABLE users (id INT, age INT, name TEXT);

-- Data Insertion
INSERT INTO users VALUES (1, 30, 'Alice');
INSERT INTO users VALUES (2, 25, 'Bob');

-- Data Querying (with WHERE conditions)
SELECT id, name FROM users WHERE id = 1;
SELECT * FROM users WHERE age > 20;

-- Data Deletion
DELETE FROM users WHERE id = 2;
```

### 3.2 4KB Slotted-Page Binary Format Layout (`btree.asm`)
```
+-------------------------------------------------------+
| Header: PageType(1B) | KeyCount(2B) | FreeSpaceOffset(2B)|
+-------------------------------------------------------+
| Slot Array: [Offset 0, Offset 1, ... Offset N]        |
+-------------------------------------------------------+
| ... Unallocated Free Space ...                        |
+-------------------------------------------------------+
| Data Heap: [Record N] ... [Record 1] [Record 0]       |
+-------------------------------------------------------+
```

---

## 4. Local Testing & Verification Engine

### 4.1 Makefile Targets (REQUIRED)
- `make build` → Assembles with `nasm -f elf64 -Werror` and links into `bin/sqlasm`.
- `make test` → Executes assembly unit tests & SQL integration scripts.
- `make lint` → Enforces `nasm -Werror` and runs `valgrind --leak-check=full --error-exitcode=1 bin/sqlasm --test`.
- `make e2e` → Runs `docker compose up --build --exit-code-from e2e-runner`.

### 4.2 Definition of Done (DoD)
1. Runs seamlessly inside Docker on ARM64 macOS via `platform: linux/amd64`.
2. Passes 100% of SQL-92 test scripts (`CREATE`, `INSERT`, `SELECT`, `DELETE`).
3. Zero memory leaks on Valgrind.
