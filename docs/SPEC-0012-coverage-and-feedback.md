# SPEC-0012 · Coverage & Feedback

> The cross-hunt coverage map every hunt feeds back into, the upsert-by-technique
> semantics that keep it one-row-per-technique, the hypothesis weighting that
> reads it to aim the next hunt, and the metric set a maturing program tracks.

| Field | Value |
|---|---|
| **ID** | SPEC-0012 |
| **Status** | Stable |
| **Updated** | 2026-06-10 |
| **Source of truth** | `internal/brain/coverage.go`, `internal/tools/update_coverage.go`, `internal/agent/prompt.go` |
| **Depends on** | SPEC-0002, SPEC-0004 |

## 1. Purpose & scope

This spec defines the **feedback** stage of the loop (stage 8) and the durable
**coverage** map it maintains: the data model, the upsert semantics, how scoping
reads coverage back, how hypothesis selection is weighted, and the metric set a
program should track as it matures.

It does **not** redefine the brain store schema (that is
[SPEC-0002](SPEC-0002-brain-and-persistence.md) §4) nor the `update_coverage`
tool contract (that is [SPEC-0004](SPEC-0004-hunting-workflow.md) R-0004-14); it
is the *why* and the *semantics* behind them.

## 2. Definitions

- **Coverage row** — one ATT&CK technique's detection-coverage state:
  `{Technique, Tactic, Status, Fidelity, Detections}` plus `updated_at` and
  relations to the hunts and detections that touched it.
- **Coverage status** — `Covered` (a reliable detection fires), `Thin` (partial
  or low-fidelity), or `Uncovered` (no detection).
- **Fidelity** — `high` | `medium` | `low` | `none`, the reliability of the
  coverage; tied to the hunt's detection tier (tier1→high, tier2→medium,
  tier3→low, tier4/5→none).
- **Feedback stage** — stage 8 of the loop: a concluded hunt upserts its
  technique's coverage and queues any follow-on hypotheses, so coverage and the
  backlog compound across hunts.

## 3. Requirements

- **R-0012-01** The coverage map MUST hold at most one row per ATT&CK technique.
  `UpsertCoverage` MUST query by `technique`, update the matching row when one
  exists, and create a new row otherwise; the match MUST be on the exact
  technique value, not a substring, so an upsert does not collide.
- **R-0012-02** Every `UpsertCoverage` write MUST stamp `updated_at` (RFC 3339)
  and MUST write only the supplied fields (technique + status are required;
  tactic, fidelity, detections are written only when non-empty).
- **R-0012-03** The feedback stage SHOULD run after every concluded hunt:
  `update_coverage` records the technique's state and `queue_hunt` parks any
  follow-on hypotheses. `store_hunt`'s lint MUST require either a coverage update
  or at least one recorded next step (see
  [SPEC-0004](SPEC-0004-hunting-workflow.md) R-0004-11d).
- **R-0012-04** The coverage map MUST be readable back via `recall` scope
  `coverage`, surfacing the technique, status, and fidelity of matching rows.
- **R-0012-05** When scoping the next hunt, the agent SHOULD weight its choice in
  strict priority order: **detection coverage gaps first** (thin/uncovered
  techniques from the coverage map), **then threat intel** active against
  similar orgs, **then environment context** (this stack's assets and risk). The
  system prompt MUST express this ordering.

## 4. Behavior & interfaces

### The coverage store

The `coverage` store (`DBCoverage`) is the seventh brain store. Its schema is
specified in [SPEC-0002](SPEC-0002-brain-and-persistence.md) §4; the title column
is `technique` (an ATT&CK ID, e.g. `attack.t1562.001`), `status` is a seeded
status column (`Covered`/`Thin`/`Uncovered`), and it relates to the `hunts` and
`detections` that touched the technique.

### Upsert-by-technique

```
UpsertCoverage(cov):
  props = {technique, updated_at=now, [tactic], [status], [fidelity], [detections]}
  id = findCoverage(cov.Technique)   # exact-match query over the coverage store
  if id != "":  UpdateRow(id, props) -> id
  else:         CreateRow(coverage, props) -> new id
```

`findCoverage` queries the store for the technique and returns the row whose
`technique` prop equals it exactly. Because the brain backends are db-name
agnostic, coverage works on `Mem`, `File`, and `NTN` with no extra plumbing
beyond the `Coverage` field on `DBIDs`.

### Reading coverage back

`recall` with `scope: coverage` lists matching coverage rows as
`technique · status · fidelity` lines. Empty-query recall lists the most
recently updated techniques; a query matches techniques/tactics by substring.
This is the scoping move that turns the coverage map into the next hypothesis.

### Hypothesis weighting

The scope-&-prioritize stage weights hypothesis selection: **coverage gaps >
threat intel > environment context**. Thin and uncovered techniques are the
strongest candidates; intel active against peer orgs is next; the environment's
crown-jewel assets and stack break ties. The framing lives in the system prompt
(`internal/agent/prompt.go`).

### Suggested metrics

These are the measures a maturing hunting program tracks. vala records the raw
material for each in the brain; computing the rollups is the operator's job
(in Notion or downstream), not a vala feature.

| Metric | What it measures | Where the raw material lives |
|---|---|---|
| Detections produced per hunt | conversion yield of the loop | `detections` rows linked to each hunt (`hunts → detections`); the hunt's `detection_tier` |
| Coverage delta | techniques newly covered or upgraded (e.g. Uncovered→Thin→Covered) | successive `coverage` rows + their `updated_at`; status/fidelity transitions |
| Visibility gaps identified & routed | blind spots found and turned into forensic-readiness work | `visibility_gap` evidence rows; the `queue_hunt` follow-ups they spawn |
| Time from hypothesis to deployed detection | loop latency | hunt `started_at`/`ended_at` and the linked detection's row timestamps |
| Hunt reproducibility | can the hunt be re-run from its evidence package | the `query`-kind evidence pointers (captured verbatim) on each hunt |
| False-positive rate of hunt-derived detections | quality of what the loop ships | tracked by the operator's SIEM against the linked detection rows; vala records the tier and `falsepositives` but does not observe production alerts |

## 5. Acceptance criteria

- **A-0012-01** (R-0012-01) `UpsertCoverage` for a technique creates a row the
  first time and updates that same row on a second call with the same technique
  (`internal/brain/coverage_test.go` `TestUpsertCoverageCreatesThenUpdates`).
- **A-0012-02** (R-0012-03) `store_hunt` is rejected when neither coverage was
  updated nor a next step recorded (`internal/brain/hunt_test.go`
  `TestLintHuntRequiresFeedback`).
- **A-0012-03** (R-0012-04) `recall` exposes a `coverage` scope and
  `recallScopes` includes the coverage store with fields technique/status/fidelity
  (`internal/tools/recall.go`).
- **A-0012-04** (R-0012-05) The system prompt enumerates the weighting order
  coverage gaps → intel → environment in the scope stage
  (`internal/agent/prompt_test.go` `TestSystemPromptEnumeratesLoopAndTiers`).
- **A-0012-05** (R-0012-01) Coverage is part of the seven-store schema and end-to-end
  loop (`internal/tools/peak_e2e_test.go` `TestPEAKLoopEndToEnd`).

## 6. Non-goals

- **No automatic coverage derivation.** Coverage status/fidelity are asserted by
  the agent at the feedback stage, not computed from linked detections.
- **No metric computation.** vala records the raw material; the rollups above are
  the operator's to compute. There is no metrics dashboard inside vala.
- **No production telemetry.** vala does not observe SIEM alerts, so the
  false-positive-rate metric is measured outside vala.

## 7. Open questions

- Should coverage `status` be derived from the relation to validated detections
  rather than asserted by the agent?
- Should vala surface a coverage summary (a count of covered/thin/uncovered per
  tactic) as a first-class read, distinct from `recall`?

## 8. References

- [SPEC-0002](SPEC-0002-brain-and-persistence.md) §4 — the `coverage` store schema.
- [SPEC-0004](SPEC-0004-hunting-workflow.md) — `update_coverage` and `recall` contracts.
- [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) — the loop and the detection-output hierarchy.
- `internal/brain/coverage.go` — `UpsertCoverage`, `findCoverage`.
- `internal/tools/update_coverage.go` — the feedback-stage tool.
