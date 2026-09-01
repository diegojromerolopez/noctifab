# ninline Specification: Generalized N-in-a-Line (M,N,K)-Game CLI & Engine in Python

## 1. Overview

`ninline` is a clean, modular, zero-runtime-dependency Python 3 command-line game engine and interactive CLI for **generalized $N$-in-a-line games** (known mathematically as $(m,n,k)$-games).

On a grid of $M$ rows $\times N$ columns, two players (`X` and `O`) take turns placing their mark. The first player to achieve $K$ consecutive marks in an unbroken straight line (horizontally, vertically, or diagonally) wins the game. If all grid cells are filled without any player achieving $K$ in a line, the game ends in a draw.

`ninline` serves as a dual-mode tool:
1. **Interactive ANSI Game**: Beautiful terminal board renderer with keyboard/coordinate move input, two-player local mode, and AI opponent mode.
2. **Headless CLI / JSON Engine**: Deterministic CLI subcommands (`eval`, `move`, `best-move`, `info`) designed for programmatic integration, testing, and script pipelines.

---

## 2. Directory Layout & Package Structure

```
ninline/
├── Makefile                 # build, test, lint, format, clean targets (REQUIRED)
├── pyproject.toml           # Package metadata, tool.ruff, tool.mypy, tool.pytest configurations
├── README.md                # Usage instructions, CLI reference, game rules
├── .gitignore               # Ignore __pycache__/, .pytest_cache/, .mypy_cache/, .ruff_cache/, dist/
├── src/
│   └── ninline/
│       ├── __init__.py      # Package version and public exports
│       ├── cli.py           # CLI entrypoint with argparse subcommands (play, eval, move, best-move, info)
│       ├── board.py         # Board domain model, serialization, boundary validation, coordinate mapping
│       ├── engine.py        # Ray-casting line scanner, win/draw detection, game state evaluator
│       ├── ai.py            # Minimax AI with alpha-beta pruning and heuristic evaluation for large grids
│       └── renderer.py      # ANSI terminal board renderer with coordinate labels & winning line highlight
└── tests/
    ├── __init__.py
    ├── test_board.py        # Unit tests: grid creation, move placement, bounds checking, string serialization
    ├── test_engine.py       # Unit tests: ray-casting win detection (horizontal, vertical, diagonal, antidiagonal, draws)
    ├── test_ai.py           # Unit tests: AI winning move completion, opponent block detection
    └── test_cli.py          # E2E & black-box CLI tests: subcommands, JSON output schema, error exit codes
```

---

## 3. Build, Invocation, and Quality Gates

### 3.1 Makefile Targets (All REQUIRED)
- `make build` → validates package setup and creates executable wrapper or wheel.
- `make test` → runs unit and CLI tests via `pytest -v tests/` (must pass 100% with zero failures).
- `make lint` → runs static analysis via `ruff check .` and `mypy --strict src/` (must pass with ZERO warnings/errors).
- `make format` → runs `ruff format --check .` (or `ruff format .` to format idempotently).
- `make clean` → removes build artifacts, cache directories, and temporary files.

### 3.2 Linter & Type Safety Gates
- **`ruff`**: Must adhere to strict style rules without suppression comments.
- **`mypy`**: Must pass `--strict` mode (all functions, methods, and return values explicitly typed).
- **Standard Library Only**: Runtime implementation in `src/ninline/` MUST NOT require third-party libraries (uses Python standard library: `argparse`, `json`, `dataclasses`, `enum`, `typing`, `sys`, `random`).

---

## 4. CLI Contract & Subcommands

### 4.1 `ninline play` (Interactive Terminal Mode)
Starts an interactive terminal session with ANSI box-drawing board layout.

```bash
ninline play [--rows M] [--cols N] [--win-length K] [--ai {none,easy,hard}] [--first {X,O}]
```

- `--rows M` (int, default: 3, minimum: 3, maximum: 50): Number of rows.
- `--cols N` (int, default: 3, minimum: 3, maximum: 50): Number of columns.
- `--win-length K` (int, default: 3, minimum: 3): Winning line length $K \le \max(M, N)$.
- `--ai {none,easy,hard}` (default: `none`): AI opponent mode.
- `--first {X,O}` (default: `X`): Player to move first.

### 4.2 `ninline eval` (Headless Board Evaluation)
Evaluates the current state of a serialized board and outputs structured JSON to `stdout`.

```bash
ninline eval --board "<SERIALIZED_BOARD>" --win-length K
```

**Board Serialization Format**:
Rows are separated by `|`. Each cell is `X`, `O`, or `.` (empty). Example for $3 \times 3$:
`"X.O|.X.|..X"`

**JSON Output Format**:
```json
{
  "status": "WIN",
  "winner": "X",
  "winning_line": [[0, 0], [1, 1], [2, 2]],
  "is_terminal": true,
  "move_count": 4,
  "rows": 3,
  "cols": 3,
  "win_length": 3
}
```

Status outcomes: `"IN_PROGRESS"`, `"WIN"`, `"DRAW"`, `"INVALID"`.

### 4.3 `ninline move` (Apply Move)
Applies a move to a serialized board state and returns the updated board state in JSON.

```bash
ninline move --board "<SERIALIZED_BOARD>" --win-length K --player {X,O} --row R --col C
```

**JSON Output Format**:
```json
{
  "success": true,
  "board": "X.O|.X.|..X",
  "status": "WIN",
  "winner": "X",
  "is_terminal": true,
  "error": null
}
```

If the move is invalid (out of bounds or cell already occupied), exit code is `1` and JSON output has `"success": false, "error": "<reason>"`.

### 4.4 `ninline best-move` (AI Move Recommendation)
Calculates the recommended next move for the active player.

```bash
ninline best-move --board "<SERIALIZED_BOARD>" --win-length K --player {X,O} [--difficulty {easy,hard}]
```

**JSON Output Format**:
```json
{
  "row": 1,
  "col": 1,
  "player": "X",
  "evaluation_score": 100
}
```

### 4.5 `ninline info` (Grid Information & Capabilities)
Prints game metrics for given $(M,N,K)$ parameters.

```bash
ninline info --rows M --cols N --win-length K
```

**JSON Output Format**:
```json
{
  "rows": 6,
  "cols": 7,
  "win_length": 4,
  "total_cells": 42,
  "total_winning_lines": 69,
  "horizontal_lines": 24,
  "vertical_lines": 21,
  "diagonal_lines": 24
}
```

---

## 5. Game Engine Requirements & Algorithms

1. **Ray-Casting Win Scanner (`engine.py`)**:
   - Direction vectors:
     - Horizontal: `(0, 1)`
     - Vertical: `(1, 0)`
     - Diagonal Down-Right: `(1, 1)`
     - Diagonal Up-Right: `(-1, 1)`
   - Computes uninterrupted consecutive sequences of length $\ge K$.
   - Highlights exact coordinates of the winning line upon victory.

2. **Move Validation (`board.py`)**:
   - Zero-indexed row and column coordinates: $0 \le r < M$, $0 \le c < N$.
   - Strict turn-order checking: Player marks must not exceed a difference of 1 (`count(X) == count(O)` or `count(X) == count(O) + 1`).

3. **AI Strategy (`ai.py`)**:
   - For small state spaces ($3 \times 3$): Exact Minimax with Alpha-Beta pruning.
   - For larger grids ($> 4 \times 4$): Depth-limited heuristic search evaluating open $K-1$ and $K-2$ chains, center proximity, and immediate winning/blocking moves.

---

## 6. Exit Codes

- `0`: Success / Valid computation.
- `1`: Invalid move, out-of-bounds coordinate, or malformed board string.
- `2`: CLI argument parse error (invalid flags).
