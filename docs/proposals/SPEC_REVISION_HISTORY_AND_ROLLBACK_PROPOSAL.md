# Noctifab Specification Revision History, Snapshot Storage & Rollback Engine

> **Document Type**: Architecture Design & Technical Proposal  
> **Target Version**: `0.44.0`  
> **Status**: Proposed  
> **Author**: Staff AI Architecture & Systems Engineering  
> **Scope**: Immutable Specification Snapshot Storage, Multi-Turn Time Travel (`undo`/`rollback`/`checkout`), Colored Revision Diffing, CLI REPL Control, and Web Dashboard Visual Timeline.

---

## 1. Executive Summary & Problem Statement

In the Human-in-the-Loop (HITL) specification design cycle, engineering requirements are rarely linear. Developers frequently explore divergent design paths:
* *"What if we switch from REST to gRPC with Protobuf schemas?"*
* *"Try adding distributed Raft consensus instead of standalone master-slave replication."*
* *"Let's test an event-driven Kafka architecture instead of Redis pub/sub."*

If an experimental refinement turn turns out to be overly complex or introduces unwanted constraints, developers need a **safe, zero-friction mechanism to roll back to earlier drafts**.

Without persistent revision snapshots:
1. **Token & Cost Waste**: Developers must spend additional LLM tokens asking the AI to *"revert what you just did"*, which frequently suffers from context drift, partial reverts, and hallucinated omissions.
2. **Loss of Previous State**: Once `SPEC.md` is overwritten, the previous version is lost unless manually backed up.
3. **No Audit Trail**: Downstream development cannot trace why specific architectural invariants or Definition of Done clauses were added or changed across iterations.

This proposal specifies an **immutable snapshot storage engine**, a **time-travel rollback system (`undo`/`checkout`)**, and a **visual revision scrubber in the Web Dashboard**.

---

## 2. Storage Topology & Snapshot Architecture

All specification snapshots are persisted in the workspace's local runtime data directory under `.noctifab/data/specs/`:

```
.noctifab/
└── data/
    └── specs/
        ├── session_1724098200.json        # Stateful session metadata, turn logs, token telemetry
        ├── revisions/
        │   ├── SPEC.v1.md                 # Immutable snapshot of Revision 1 (Initial Draft)
        │   ├── SPEC.v2.md                 # Immutable snapshot of Revision 2 (Feedback 1: Add TLS)
        │   ├── SPEC.v3.md                 # Immutable snapshot of Revision 3 (Feedback 2: gRPC probe)
        │   └── SPEC.v4.md                 # Immutable snapshot of Revision 4 (Rollback to v2 + Prometheus)
        └── diffs/
            ├── diff_v1_to_v2.patch        # Unified line-by-line patch from v1 to v2
            ├── diff_v2_to_v3.patch        # Unified line-by-line patch from v2 to v3
            └── diff_v3_to_v4.patch        # Unified line-by-line patch from v3 to v4
```

### 2.1. Snapshot Invariants:
1. **Immutability**: Once a revision file (`SPEC.v<N>.md`) is written to disk, it is never modified.
2. **Deterministic Hash**: Each snapshot includes a SHA-256 content hash in the session JSON metadata.
3. **Clean Workspace Isolation**: The `revisions/` and `diffs/` directories reside in `.noctifab/data/` which is ignored by `.noctifab/.gitignore`, ensuring repository cleanliness.

---

## 3. Domain Model Extensions (`pkg/domain/spec_session.go`)

The `SpecSession` and `SpecRevision` domain models are extended with revision pointers, SHA-256 hashes, and time-travel operations:

```go
package domain

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// SpecRevision represents an immutable snapshot in the specification revision tree.
type SpecRevision struct {
	Version      int               `json:"version"`
	SnapshotPath string            `json:"snapshot_path"`
	SHA256       string            `json:"sha256"`
	Prompt       string            `json:"prompt"`
	Kind         SpecTurnKind      `json:"kind"` // INITIAL, REFINE, ROLLBACK, AUDIT
	ParentVer    int               `json:"parent_version"`
	DiffSummary  string            `json:"diff_summary,omitempty"`
	TokensUsed   int64             `json:"tokens_used"`
	CostUSD      string            `json:"cost_usd"`
	ModelRoles   map[string]string `json:"model_roles,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

// SpecSession manages stateful multi-turn HITL sessions with time-travel capabilities.
type SpecSession struct {
	ID             string         `json:"id"`
	ProjectPath    string         `json:"project_path"`
	TargetFile     string         `json:"target_file"`
	ActiveVerIndex int            `json:"active_version_index"` // Pointer to current active version
	Revisions      []SpecRevision `json:"revisions"`
	CurrentSpec    string         `json:"current_spec"`
	IsComplete     bool           `json:"is_complete"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// ActiveRevision returns the active revision snapshot.
func (s *SpecSession) ActiveRevision() *SpecRevision {
	if len(s.Revisions) == 0 || s.ActiveVerIndex < 0 || s.ActiveVerIndex >= len(s.Revisions) {
		return nil
	}
	return &s.Revisions[s.ActiveVerIndex]
}

// CanUndo returns true if a previous historical version exists.
func (s *SpecSession) CanUndo() bool {
	return s.ActiveVerIndex > 0
}

// CanRedo returns true if a forward revision exists after an undo.
func (s *SpecSession) CanRedo() bool {
	return s.ActiveVerIndex < len(s.Revisions)-1
}
```

---

## 4. Interactive CLI & REPL Time-Travel Commands

During the interactive `noctifab spec` loop, developers have access to dedicated time-travel steering commands:

### 4.1. Supported REPL Commands:

| Command | Shorthand | Description | Token Cost |
|---|---|---|---|
| `undo` | `u` | Reverts the current draft to the previous revision (`SPEC.v(N-1).md`) | **0 Tokens** (Instant) |
| `redo` | `r` | Restores the forward revision if an undo was previously performed | **0 Tokens** (Instant) |
| `history` | `log` | Displays a visual timeline of all revisions, prompts, and line changes | **0 Tokens** |
| `checkout <v>` | `co <v>` | Jumps directly to a specific revision (e.g. `checkout 2` or `co v1`) | **0 Tokens** (Instant) |
| `diff <vA> <vB>`| | Displays side-by-side or colored line diff between two revisions | **0 Tokens** |

### 4.2. Example Terminal Interaction:

```
────────────────────────────────────────────────────────────────────────────────
📄 Current SPEC.md Draft (Revision 3 | 240 lines)
────────────────────────────────────────────────────────────────────────────────
... [draft preview showing gRPC microservice schemas] ...

[Turn 3] What would you like to improve, fix, or add?
(Type your instructions, or say 'undo', 'history', 'looks good' to approve)
> history

📜 Specification Revision Timeline:
  • v1 (Turn 1) [142 lines] Initial REST API Draft (PM: Claude, Arch: GPT-4o)
  • v2 (Turn 2) [186 lines] Added TLS certificates and Prometheus metrics (+44 lines)
  * v3 (Turn 3) [240 lines] (ACTIVE) Converted to gRPC microservice schemas (+54 lines)

[Turn 3] What would you like to improve, fix, or add?
> undo

⏪ Rolled back to Revision 2 (SPEC.v2.md).
✔ Restored 186-line specification from snapshot cache (0 tokens used).

[Turn 4] What would you like to improve, fix, or add?
> "Keep the REST API from v2, but add JWT authentication with refresh token rotation"
```

---

## 5. Visual Web Dashboard Timeline Scrubber (`/spec`)

In the Web Dashboard Spec Studio (`http://127.0.0.1:8080`), the revision history is presented as an interactive visual timeline:

```
 ┌─────────────────────────────────────────────────────────────────────────────┐
 │  📄 SPEC.md Studio  │  ● v1 (Draft) ── ● v2 (TLS+Metrics) ── ● v3 (JWT Auth)│
 └─────────────────────────────────────────────────────────────────────────────┘
  [ ⏪ Undo ]  [ ⏩ Redo ]  [ 📜 View History ]  [ 🔍 Diff with v2 ]  [ ✅ Approve ]
```

### Visual Features:
1. **Interactive Revision Pills**: Clicking on `v1`, `v2`, or `v3` immediately displays that exact immutable snapshot in the left editor pane without calling LLMs.
2. **Visual Diff Inspector**: Toggle between "Single Document View" and "Side-by-Side Diff View" to see exact additions (`+`) and deletions (`-`) between any two versions.
3. **Prompt Replay**: Hovering over a version pill reveals the exact user prompt and model attributions that generated it.
4. **"Restore This Version"**: One-click restore button sets that historical revision as the base for all subsequent prompt refinements.

---

## 6. Implementation Milestones & Roadmap

| Milestone | Deliverables | Target Packages |
|---|---|---|
| **Phase 1: Storage & Snapshot Engine** | Implement `SpecSnapshotManager` to write/read immutable `SPEC.v<N>.md` and compute SHA-256 hashes. | `pkg/infrastructure/storage/spec_snapshot.go` |
| **Phase 2: Domain Time-Travel Logic** | Implement `Undo()`, `Redo()`, and `Checkout()` on `SpecSession` with unit tests. | `pkg/domain/spec_session.go`, `pkg/domain/spec_session_test.go` |
| **Phase 3: REPL Command Parser** | Integrate `undo`, `redo`, `history`, and `checkout` into `SpecOrchestrator` and `SpecRenderer`. | `pkg/services/spec_orchestrator.go`, `pkg/services/spec_renderer.go` |
| **Phase 4: Web API & Frontend Timeline** | Expose `GET /api/v1/spec/history` and `POST /api/v1/spec/checkout/:version`, adding the interactive visual scrubber in `app.js` and `styles.css`. | `pkg/interfaces/web/spec_handler.go`, `pkg/interfaces/web/static/` |

---

## 7. Conclusion

By saving immutable snapshots (`SPEC.v1.md`, `SPEC.v2.md`...) in `.noctifab/data/specs/revisions/`, Noctifab gives developers complete peace of mind to experiment with bold architectural ideas during the specification phase, knowing they can instantly time-travel back with zero token costs and zero latency.
