# Language Sandbox Templates

Noctifab enables you to customize and share language-specific sandbox execution environments across your projects without editing global codebase logic. 

Templates define the build tools, test runners, AST code formatters, static analysis linters, and binary execution whitelists that govern the autonomous dark factory loop for each programming language.

---

## 1. Resolution Hierarchy

When a template is referenced (e.g., via `noctifab init -t <name>`, `noctifab templates show <name>`, or auto-detection), Noctifab searches for `<name>.yaml` or `<name>.yml` in the following order (first match wins):

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Workspace Tier: <project>/.noctifab/templates/<name>.yaml │ (Highest Priority)
└──────────────────────────────┬──────────────────────────────┘
                               │ (fallback if missing)
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. User Global Tier: ~/.noctifab/templates/<name>.yaml      │ (User-Level Defaults)
└──────────────────────────────┬──────────────────────────────┘
                               │ (fallback if missing)
                               ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Built-in Tier: Embedded defaults compiled into noctifab  │ (Standard Fallback)
└─────────────────────────────────────────────────────────────┘
```

1. **Workspace Tier (`.noctifab/templates/<name>.yaml`)**: Checked into the repository to guarantee identical test runners and linter flags across your entire team and CI/CD pipelines.
2. **User Global Tier (`~/.noctifab/templates/<name>.yaml`)**: Stored in your home directory to apply your personal preferences (such as using `ruff` vs `flake8` or `cargo-nextest` vs `cargo test`) across all local projects.
3. **Built-in Tier**: Embedded default configurations for 10 major programming languages shipped inside the `noctifab` binary.

---

## 2. Built-in Templates Catalog

Noctifab ships with battle-tested configurations verified across the 20 validation projects:

| Canonical Name | Aliases | Test Command | Formatter Command | Linter Command | Whitelisted Binaries |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **`go`** | `golang` | `go test -v ./...` | `go fmt ./...` | `go vet ./...` | `go`, `git`, `make`, `docker` |
| **`python`** | `py` | `pytest` | `ruff format .` | `ruff check . && mypy --strict .` | `python3`, `python`, `pip`, `pytest`, `coverage`, `ruff`, `mypy`, `git`, `make` |
| **`rust`** | `rs` | `cargo test --workspace` | `cargo fmt` | `cargo fmt --check && cargo clippy -- -D warnings` | `cargo`, `rustc`, `gcc`, `clang`, `git`, `make` |
| **`typescript`** | `ts`, `js`, `javascript` | `npm test` | `npx prettier --write .` | `npx eslint . && npx tsc --noEmit` | `npm`, `npx`, `node`, `tsc`, `vitest`, `eslint`, `prettier`, `git`, `make` |
| **`c`** | `cpp`, `c++` | `make test` | `make format` | `make lint` | `make`, `gcc`, `clang`, `clang-format`, `clang-tidy`, `sqlite3`, `git` |
| **`ruby`** | `rb` | `bundle exec rspec` | `bundle exec rubocop -a` | `bundle exec rubocop` | `ruby`, `bundle`, `rspec`, `rubocop`, `git`, `make` |
| **`csharp`** | `cs`, `c#`, `dotnet` | `dotnet test` | `dotnet format` | `dotnet build --warnaserror` | `dotnet`, `git`, `make` |
| **`java`** | — | `./gradlew test` | `./gradlew spotlessApply` | `./gradlew check` | `gradle`, `./gradlew`, `java`, `javac`, `git`, `make` |
| **`erlang`** | `otp` | `rebar3 eunit` | — | `rebar3 dialyzer` | `rebar3`, `erl`, `erlc`, `git`, `make` |
| **`ocaml`** | `ml` | `dune runtest` | `dune fmt` | `dune build @check` | `dune`, `ocamlc`, `ocamlopt`, `git`, `make` |

---

## 3. Anatomy of a Template File

A template file is a YAML document declaring a partial or complete `sandbox` configuration:

```yaml
sandbox:
  mode: host                 # Execution mode: "host" or "docker"
  timeout_seconds: 300       # Per-command execution timeout in seconds
  test_command: pytest       # Command executed by Tester and Quality Gates
  formatter_command: ruff format . # Local formatting command for deterministic AST styling
  linter:
    command: ruff check . && mypy --strict . # Static analysis checks
    max_issues: 100          # Maximum issues before aborting turn
  allowed_commands:          # Strict security whitelist of permitted CLI binaries
    - python3
    - python
    - pip
    - pytest
    - coverage
    - ruff
    - mypy
    - git
    - make
  forbidden_patterns:        # Optional regex patterns rejected deterministically
    - \bimport\s+pdb\b
```

### Key Field Descriptions:
* **`test_command`**: The command used to run tests. Must output empirical results to stdout/stderr.
* **`formatter_command`**: A safe local formatter (e.g. `gofmt`, `rustfmt`, `prettier`, `ruff`). Never run linters in this field.
* **`linter.command`**: Command that inspects code quality and exits non-zero if issues exist.
* **`allowed_commands`**: **Mandatory security boundary.** Any command or sub-process invocation not present in this list is blocked by Noctifab to prevent command injection or shell escapes.
* **`forbidden_patterns`**: (Optional) Regular expressions that trigger immediate AST validation rejection if detected in generated source code (e.g. rejecting `unsafe` blocks in Rust).

---

## 4. Managing Templates via the CLI

Noctifab provides the `templates` command suite to inspect, export, and customize templates.

### 4.1 List All Available Templates (`noctifab templates list`)
Shows all discovered templates and where each is resolved from:
```bash
noctifab templates list
```
*Output:*
```text
Available language templates:
  NAME           SOURCE       DESCRIPTION
  c              builtin      C17 / C++ with make test, make format, and make lint
  go             builtin      Go 1.22+ with go test, go fmt, and go vet
  python         workspace    Workspace template (.noctifab/templates/python.yaml)
  rust           global       User global template (~/.noctifab/templates/rust.yaml)
  typescript     builtin      TypeScript / Node.js with npm test, prettier, eslint, and tsc
```

### 4.2 Inspect a Template (`noctifab templates show <name>`)
Prints the effective YAML content and source path:
```bash
noctifab templates show python
```

### 4.3 Export Editable Starting Points (`noctifab templates init`)
Export built-in templates to disk so you can modify them:

```bash
# Export python template into current workspace (.noctifab/templates/python.yaml)
noctifab templates init python

# Export rust template into your user global config (~/.noctifab/templates/rust.yaml)
noctifab templates init rust --global

# Export all 10 built-in templates into the workspace at once
noctifab templates init
```

---

## 5. Using Templates with `noctifab init`

When creating or bootstrapping a new project workspace, use the `--template` (or `-t`) flag:

```bash
# Initialize a new workspace explicitly using the Python template
noctifab init my-api --template python

# Short flag form
noctifab init my-service -t rust
```

### Auto-Detection Behavior:
If `--template` is omitted, `noctifab init` automatically:
1. Detects the project language from manifest files (`Cargo.toml`, `pyproject.toml`, `package.json`, etc.).
2. Checks if a custom template exists in `.noctifab/templates/<language>.yaml` or `~/.noctifab/templates/<language>.yaml`. If found, it applies your custom template automatically!
3. If no custom template exists, it applies standard manifest heuristics (`go test`, `npm test`, `pytest`).

---

## 6. How-To Guides: Customizing Templates

### Example 1: Custom Python Template with Flake8 and Black
If you prefer `black` and `flake8` over `ruff`, create `~/.noctifab/templates/python.yaml`:

```yaml
sandbox:
  mode: host
  timeout_seconds: 300
  test_command: pytest -v
  formatter_command: black .
  linter:
    command: flake8 . && mypy .
    max_issues: 50
  allowed_commands:
    - python3
    - python
    - pip
    - pytest
    - black
    - flake8
    - mypy
    - git
    - make
```
Now, any time you run `noctifab init --template python` or initialize a Python project, Noctifab will use your custom toolchain.

### Example 2: Adding a Brand New Language (e.g. Zig)
You can create templates for languages that are not built into Noctifab. Save this as `~/.noctifab/templates/zig.yaml` or `.noctifab/templates/zig.yaml`:

```yaml
sandbox:
  mode: host
  timeout_seconds: 300
  test_command: zig test src/main.zig
  formatter_command: zig fmt .
  linter:
    command: zig build-exe src/main.zig
    max_issues: 10
  allowed_commands:
    - zig
    - git
    - make
```
Then initialize your Zig project:
```bash
noctifab init my-zig-app --template zig
```

### Example 3: Team-Shared Workspace Template
To ensure everyone on your team and your GitHub Actions CI use the exact same test runner settings, commit your template to Git:

```bash
mkdir -p .noctifab/templates
cp my-custom-go.yaml .noctifab/templates/go.yaml
git add .noctifab/templates/go.yaml
git commit -m "chore: pin project sandbox template for Go"
```
Because the workspace tier has the highest priority, all team members running `noctifab` in this repository will use `.noctifab/templates/go.yaml`.
