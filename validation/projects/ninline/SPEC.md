# ninline Specification: Generalized (M,N,K)-Game CLI & Engine in Python

> **Goal**: High-performance, zero-dependency Python 3 CLI game engine for generalized $N$-in-a-line ($(m,n,k)$-games) on an $M \times N$ grid with target line length $K$. Supports interactive ANSI terminal play and deterministic headless JSON subcommands.

---

## ⚡ 1. TL;DR & Core System Invariants

* **Language**: Python 3 (Strict type annotations verified by `mypy --strict`, formatted by `ruff`).
* **Dependencies**: Python standard library ONLY (`argparse`, `json`, `dataclasses`, `enum`, `typing`, `sys`, `random`).
* **Game Rules**:
  * Grid dimensions: $M$ rows $\times N$ columns ($3 \le M, N \le 50$).
  * Target length: $K$ consecutive marks in a straight line to win ($3 \le K \le \max(M, N)$).
  * Players: `X` (First player) and `O` (Second player).
  * Cell states: `X`, `O`, or `.` (empty).
  * Game states: `"IN_PROGRESS"`, `"WIN"`, `"DRAW"`, `"INVALID"`.

---

## 📁 2. Pinned Directory Layout

```text
ninline/
├── Makefile                 # build, test, lint, format, clean (REQUIRED)
├── pyproject.toml           # ruff, mypy, pytest configs
├── README.md                # usage & API contract
├── .gitignore               # __pycache__, .pytest_cache, .mypy_cache
├── src/
│   └── ninline/
│       ├── __init__.py      # Package version & exports
│       ├── cli.py           # CLI dispatch (play, eval, move, best-move, info)
│       ├── board.py         # Board domain model, serialization, boundary checks
│       ├── engine.py        # Ray-casting line scanner, win/draw evaluator
│       ├── ai.py            # Alpha-Beta Minimax AI & heuristic search
│       └── renderer.py      # ANSI terminal box-drawing board renderer
└── tests/
    ├── test_board.py        # Grid creation, move placement, bounds, serialization
    ├── test_engine.py       # Ray-casting win detection (H, V, Diagonals, Draws)
    ├── test_ai.py           # Immediate win completion & block detection
    └── test_cli.py          # E2E CLI subcommands, JSON schemas, exit codes
```

---

## 🛠️ 3. Quality & Build Commands (Makefile)

* `make build` → Validates package structure and executable entrypoint.
* `make test` → Runs all tests via `pytest -v tests/` (100% pass, 0 failures).
* `make lint` → Runs `ruff check .` and `mypy --strict src/` (0 warnings, 0 errors).
* `make format` → Runs `ruff format --check .` (idempotent code formatting).
* `make clean` → Removes `__pycache__`, `.pytest_cache`, and build artifacts.

---

## 🕹️ 4. CLI Subcommand Contracts (Deterministic JSON stdout)

### 4.1 Board Serialization Invariant
* Rows are joined by `|`.
* Example $3 \times 3$ grid: `"X.O|.X.|..X"`

### 4.2 `ninline eval` (Evaluate Board State)
```bash
ninline eval --board "X.O|.X.|..X" --win-length 3
```
* **Output Format (stdout)**:
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

### 4.3 `ninline move` (Apply Single Move)
```bash
ninline move --board "X.O|...|..." --win-length 3 --player O --row 1 --col 1
```
* **Output Format (stdout)**:
```json
{
  "success": true,
  "board": "X.O|.O.|...",
  "status": "IN_PROGRESS",
  "winner": null,
  "is_terminal": false,
  "error": null
}
```
* If move is invalid (occupied or out of bounds): exits `1` with `{"success": false, "error": "<reason>"}`.

### 4.4 `ninline best-move` (AI Move Recommender)
```bash
ninline best-move --board "XX.|OO.|..." --win-length 3 --player X --difficulty hard
```
* **Output Format (stdout)**:
```json
{
  "row": 0,
  "col": 2,
  "player": "X",
  "evaluation_score": 100
}
```

### 4.5 `ninline info` (Board Metrics)
```bash
ninline info --rows 6 --cols 7 --win-length 4
```
* **Output Format (stdout)**:
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

### 4.6 `ninline play` (Interactive ANSI Game)
```bash
ninline play --rows 3 --cols 3 --win-length 3 --ai easy --first X
```

---

## 🧠 5. Algorithms & Implementation Details

1. **Ray-Casting Win Scanner (`engine.py`)**:
   * Scan 4 direction vectors from every non-empty cell:
     * Horizontal: `(0, 1)`
     * Vertical: `(1, 0)`
     * Diagonal Main: `(1, 1)`
     * Diagonal Anti: `(-1, 1)`
   * Check bounds: $0 \le r + k \cdot dr < M$ and $0 \le c + k \cdot dc < N$.
2. **AI Decision Engine (`ai.py`)**:
   * Exact Minimax with Alpha-Beta pruning for boards with $\le 9$ open cells.
   * Depth-limited search ($\text{depth} \le 4$) with weighted pattern scoring (immediate wins, opponent blocks, center control) for larger boards.
3. **Exit Codes**:
   * `0`: Normal completion / successful computation.
   * `1`: Invalid move or illegal board state.
   * `2`: CLI argument parse error.
