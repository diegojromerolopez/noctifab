# dotchess Specification: UCI Chess Engine with Stockfish Perft Validation in .NET 9

## 1. Overview

`dotchess` is a high-performance **Universal Chess Interface (UCI)** compliant chess engine written in **C# 13 / .NET 9**, runnable on Linux, and rigorously verified against the canonical computer chess validation suites used by **Stockfish** and the wider chess programming community (in particular, the **Perft benchmark suite**).

The engine executes as a headless command-line binary communicating via standard input/output according to the UCI specification. It combines:
- **Bitboard Engine:** 64-bit unsigned integers (`ulong`) representing piece placements and attack boards for high-speed bitwise move generation.
- **Strict FIDE Rules Compliance:** Complete legal move generation handling standard piece moves, double pawn pushes, en-passant captures, promotions (Queen, Rook, Bishop, Knight), and all castling permutations (with strict checks preventing castling out of check, through check, or into check).
- **Canonical Stockfish Perft Verification:** Deterministic mathematical validation against the universal Perft benchmark test suite (`test_suite/perftsuite.epd`), verifying that every legal move count matches exact theoretical invariants at every ply.
- **Alpha-Beta Search & Evaluation:** Negamax search with Alpha-Beta pruning, Quiescence search, MVV-LVA move ordering, Piece-Square Tables (PST), and Zobrist transposition table caching.
- **Multithreaded Search:** Parallel root search or Lazy SMP across threads via `System.Threading.Tasks`.

---

## 2. Pinned Directory Layout

```
dotchess/
├── Dotchess.sln              # .NET Solution file
├── Makefile                  # build/run/test/perft targets (all REQUIRED)
├── README.md                 # Usage, UCI protocol instructions, Perft benchmark documentation
├── .gitignore                # Ignore bin/, obj/, *.user
├── test_suite/
│   └── perftsuite.epd        # Canonical Stockfish Perft benchmark positions & expected node counts
├── src/
│   └── Dotchess/
│       ├── Dotchess.csproj   # C# 13, net9.0, Nullable=enable, TreatWarningsAsErrors=true
│       ├── Program.cs        # Main entrypoint: CLI command routing & UCI loop
│       ├── Engine/
│       │   ├── Bitboard.cs   # Bitwise utility operations (popcount, bitscan, ray shifts)
│       │   ├── Piece.cs      # Piece and Color representations
│       │   ├── Square.cs     # Square coordinate representations (A1..H8, 0..63)
│       │   ├── Board.cs      # Full board state, FEN parser, piece bitboards, castling/en-passant state
│       │   ├── Move.cs       # Compact 16-bit or 32-bit move representation
│       │   ├── MoveGenerator.cs # Legal move generator (pawns, knights, sliding pieces, castling)
│       │   ├── Zobrist.cs    # 64-bit Zobrist hashing for board positions
│       │   └── Perft.cs      # Performance Test leaf node counter & move breakdown
│       ├── Search/
│       │   ├── ISearchEngine.cs # Search abstraction
│       │   ├── SearchEngine.cs  # Alpha-Beta negamax, quiescence, PVS, iterative deepening
│       │   ├── Evaluation.cs    # Material balance + Piece-Square Tables (PST)
│       │   └── TranspositionTable.cs # Hash table caching positions, depth, and best moves
│       └── Uci/
│           ├── UciProtocol.cs   # Stdin/stdout command loop (uci, isready, position, go, stop)
│           └── UciParser.cs     # Move coordinate parser (e2e4, e7e8q)
└── tests/
    └── Dotchess.Tests/
        ├── Dotchess.Tests.csproj   # xUnit, Microsoft.NET.Test.Sdk
        ├── BitboardTests.cs        # Popcount, bitscan, shift invariants
        ├── FenParserTests.cs       # FEN string parsing and serialization roundtrips
        ├── MoveGeneratorTests.cs   # Move generation edge cases (en-passant, promotions, check evasions)
        ├── PerftStockfishTests.cs  # Stockfish Perft test suite (Initial, Kiwipete, Positions 3, 4, 5)
        ├── SearchTests.cs          # Mate-in-1, tactical captures, search termination
        └── UciProtocolTests.cs     # UCI command loop string input/output assertions
```

---

## 3. Toolchain, Invocation, Exit Codes, and Configuration

### 3.1 Runtime / Dev Toolchain (pinned)
- **SDK:** .NET 9.0 SDK (`mcr.microsoft.com/dotnet/sdk:9.0-alpine`).
- **Compiler Settings (`Dotchess.csproj`):**
  ```xml
  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net9.0</TargetFramework>
    <Nullable>enable</Nullable>
    <ImplicitUsings>enable</ImplicitUsings>
    <TreatWarningsAsErrors>true</TreatWarningsAsErrors>
  </PropertyGroup>
  ```

### 3.2 Makefile Targets (all REQUIRED)
- `make build` → `dotnet build -c Release` (zero compiler warnings allowed).
- `make run` → `dotnet run --project src/Dotchess -c Release`.
- `make test` → `dotnet test -c Release --verbosity normal` (runs all unit and Stockfish Perft tests; zero failures allowed).
- `make perft` → `dotnet run --project src/Dotchess -c Release -- perft 5` (executes root Perft to depth 5).
- `make clean` → `dotnet clean && rm -rf src/Dotchess/bin src/Dotchess/obj tests/Dotchess.Tests/bin tests/Dotchess.Tests/obj`.

### 3.3 CLI Invocation & Modes
The compiled binary (`dotchess`) supports two operating modes:
1. **Interactive UCI Mode (Default):**
   When invoked with no arguments (`dotchess`), the binary reads UCI commands line-by-line from `stdin` and emits responses to `stdout`.
2. **Direct CLI Perft Mode:**
   `dotchess perft <depth> [fen]`
   Executes a Perft count from the given FEN (or starting position if omitted) to `<depth>`, printing move breakdowns and total leaf nodes visited, exiting code `0`.

### 3.4 Exit Codes and Logging
- Clean UCI `quit` or `SIGINT`/`SIGTERM` → exit code `0`.
- Fatal error (e.g. invalid arguments) → write `dotchess: <error>` to stderr, exit code `1`.

---

## 4. Universal Chess Interface (UCI) Protocol Contract

`dotchess` must implement the standard UCI protocol over `stdin` and `stdout`:

| Inbound Command | Required Behavior & Outbound Response |
| :--- | :--- |
| `uci` | Responds with identity lines and `uciok`:<br>`id name dotchess`<br>`id author Noctifab`<br>`uciok` |
| `isready` | Responds with `readyok` once initialized. |
| `ucinewgame` | Clears transposition tables and resets engine state. |
| `position startpos [moves e2e4 ...]` | Sets board state to initial starting position and plays listed moves in sequence. |
| `position fen <fen> [moves ...]` | Sets board state to provided FEN and plays listed moves in sequence. |
| `go depth <N>` | Initiates search up to depth `<N>`. Periodically prints search info:<br>`info depth <d> score cp <score> nodes <n> pv <moves...>`<br>Upon completion, emits best move:<br>`bestmove <move>` (e.g. `bestmove e2e4`) |
| `go movetime <ms>` | Searches for `<ms>` milliseconds and outputs `bestmove <move>`. |
| `go perft <depth>` | Runs Perft to depth `<depth>` and outputs per-move leaf counts and `Nodes searched: <total>`. |
| `stop` | Aborts ongoing search immediately and emits `bestmove <move>`. |
| `quit` | Terminates process cleanly with exit code `0`. |

---

## 5. Stockfish Perft Benchmark Suite (The Validation Standard)

Perft (Performance Test) is the universal, mathematically invariant test of chess move generator correctness. The engine MUST match the following exact leaf node counts:

### 5.1 Standard Starting Position
FEN: `rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1`

| Depth | Exact Leaf Nodes |
| :---: | :--- |
| **1** | 20 |
| **2** | 400 |
| **3** | 8,902 |
| **4** | 197,281 |
| **5** | 4,865,609 |

### 5.2 Position 2: "Kiwipete" by Peter McKenzie
Stress tests en-passant, discovered checks, promotions, and castling safety.  
FEN: `r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1`

| Depth | Exact Leaf Nodes |
| :---: | :--- |
| **1** | 48 |
| **2** | 2,039 |
| **3** | 97,862 |
| **4** | 4,085,603 |

### 5.3 Position 3: Endgame Pawn Structure
FEN: `8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1`

| Depth | Exact Leaf Nodes |
| :---: | :--- |
| **1** | 14 |
| **2** | 191 |
| **3** | 2,812 |
| **4** | 43,238 |
| **5** | 674,624 |

### 5.4 Position 4: Mirrored Promoted Pawns
FEN: `r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1`

| Depth | Exact Leaf Nodes |
| :---: | :--- |
| **1** | 6 |
| **2** | 264 |
| **3** | 9,467 |
| **4** | 422,333 |

### 5.5 Position 5: Pinned Pieces & Tricky Checks
FEN: `rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8`

| Depth | Exact Leaf Nodes |
| :---: | :--- |
| **1** | 44 |
| **2** | 1,486 |
| **3** | 62,379 |
| **4** | 2,103,487 |

---

## 6. Search & Evaluation Architecture

1. **Evaluation Engine:**
   * Material values: Pawn (100), Knight (320), Bishop (330), Rook (500), Queen (900), King (20000).
   * Piece-Square Tables (PST) providing positional bonuses for opening/middlegame and endgame.
2. **Negamax Search with Alpha-Beta Pruning:**
   * Quiescence search at leaf nodes: continues searching captures and promotions until the position is quiet, eliminating the horizon effect.
   * Move Ordering: Searches hash move first (from transposition table), followed by MVV-LVA (Most Valuable Victim - Least Valuable Attacker) capture ordering to achieve optimal alpha-beta branch cutoffs.
3. **Transposition Table:**
   * Fixed-size hash table using 64-bit Zobrist keys.
   * Records position hash, search depth, evaluation flag (Exact, LowerBound, UpperBound), and best move.
4. **Time Management & Interruption:**
   * Listens to search cancellation tokens (`CancellationTokenSource`). When `stop` is received via UCI, search unwinds immediately and returns the best move found so far.
