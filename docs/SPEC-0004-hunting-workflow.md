# SPEC-0004 · Hunting Workflow

> The seven tools that drive the loop: recall, queue, open, record findings and
> intel, link, and conclude — each writing the brain through one session context.

| Field | Value |
|---|---|
| **ID** | SPEC-0004 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `internal/tools/{recall,queue_hunt,open_hunt,record_finding,record_intel,link_artifacts,store_hunt}.go`, `internal/tools/runcontext.go` |
| **Depends on** | SPEC-0001, SPEC-0002, SPEC-0003 |

## 1. Purpose & scope

This spec defines the hunting tools — the agent-facing primitives that move a
hunt through Scope → Hunt → Conclude → Automate and record every step in the
brain. It fixes each tool's name, input, read-only status, brain effect, and the
shared session state they write through.

It does **not** define the brain's data model (that is
[SPEC-0002](SPEC-0002-brain-and-persistence.md)) nor the detection-authoring
tools used in the Automate step (that is
[SPEC-0006](SPEC-0006-detection-authoring.md)).

## 2. Definitions

- **RunContext (`rc`)** — per-session state the hunting tools share: the active
  hunt ID and question, the brain client, and the findings recorded so far.
- **Active hunt** — the hunt `open_hunt` set as current; `record_finding` and
  `store_hunt` target it.
- **Finding ID** — the ID `record_finding` returns, which a concluding claim
  must cite.

## 3. Requirements

### Session state

- **R-0004-01** The hunting tools MUST share one `RunContext`. `open_hunt` MUST
  set the active hunt on it; `record_finding`, `record_intel`, and `store_hunt`
  MUST act on that active hunt.
- **R-0004-02** `record_finding` and `store_hunt` MUST fail clearly if no hunt
  is active, rather than writing an orphaned row.

### Tool contracts

- **R-0004-03** `recall` MUST be the only read-only hunting tool; it MUST read
  the brain back and MUST NOT write. Every other hunting tool mutates the brain
  and MUST be permission-gated.
- **R-0004-04** `recall` MUST accept a free-text `query` (empty lists recent), an
  optional `scope` over the brain stores, and an optional `limit` (default 5),
  returning compact per-store summaries.
- **R-0004-05** `queue_hunt` MUST write a backlog row in state `Queued` from a
  `trigger` + `hypothesis` (with optional behavior, data_source, priority,
  mitre) and return its ID.
- **R-0004-06** `open_hunt` MUST open a hunts row (state `Open`), set it as the
  active hunt, and return its ID. Given a `backlog_id`, it MUST mark that backlog
  item `Opened` and link it to the new hunt.
- **R-0004-07** `record_finding` MUST append an immutable evidence row to the
  active hunt and MUST return an ID for the agent to cite. It MUST require a
  `claim`, a `source` (`query` | `url` | `file_hash` | `log_ref`), and a
  `pointer`.
- **R-0004-08** `record_intel` MUST write an intel row (`indicator` | `ttp` |
  `actor` | `narrative`) and, when a hunt is active, link it to that hunt.
- **R-0004-09** `link_artifacts` MUST set a relation (`evidence` | `intel` |
  `hunts` | `detections`) on a source row to one or more target rows.
- **R-0004-10** `store_hunt` MUST close the active hunt with exactly one outcome
  (`Confirmed` | `Refuted` | `Inconclusive`), write the narrative page, and
  enforce the citation rule below.

### Citation discipline (the lint)

- **R-0004-11** `store_hunt` MUST reject the hunt page if any declarative
  (non-hypothesis) finding cites no evidence, or cites an evidence ID with no
  matching recorded finding. A claim is exempt only if explicitly marked a
  hypothesis.
- **R-0004-12** On a **Confirmed** outcome, `store_hunt` SHOULD return an
  outcome-aware directive steering the agent into authoring + linking a Sigma
  detection (or an explicit "no detection warranted" note). On **Refuted** /
  **Inconclusive** it MUST NOT push for a detection.

## 4. Behavior & interfaces

Inputs below list `required` fields in **bold**. All tools except `recall` are
non-read-only (permission-gated).

### `recall` *(read-only)*
Search the brain before opening work, so hunts compound and settled ground is
not re-hunted.
- Input: **`query`** (string; empty = recent), `scope` (`all`|`hunts`|`intel`|`detections`|`backlog`), `limit` (int, default 5).
- Reads: hunts, intel, detections, backlog. Returns compact summaries
  (hunt: question/status/behavior; intel: kind/value/confidence; detection:
  title/status/path; backlog: hypothesis/status/priority).

### `queue_hunt`
Park a trigger as a prioritized backlog hypothesis (Scope step, deferred).
- Input: **`trigger`**, **`hypothesis`**, `behavior`, `data_source`,
  `priority` (`low`|`medium`|`high`), `mitre`.
- Writes: backlog row, `status=Queued`. Returns backlog ID.

### `open_hunt`
Open a hypothesis-driven hunt and make it active (Hunt step entry).
- Input: **`question`**, `hypothesis`, `behavior`, `data_source`, `mitre`,
  `backlog_id`.
- Writes: hunts row, `status=Open`; sets `rc.HuntID`. With `backlog_id`: sets
  that backlog item `Opened` and links it. Returns hunt ID.

### `record_finding`
Record an immutable pointer that backs a claim.
- Input: **`claim`**, **`source`** (`query`|`url`|`file_hash`|`log_ref`),
  **`pointer`**, `confidence` (`confirmed`|`probable`|`hypothesis`, default
  `probable`).
- Writes: evidence row linked to the active hunt. Returns finding ID **to cite**.

### `record_intel`
Surface reusable intelligence.
- Input: **`kind`** (`indicator`|`ttp`|`actor`|`narrative`), **`value`**,
  `mitre`, `confidence`, `source`, `description`.
- Writes: intel row; auto-links to the active hunt if one is open. Returns intel ID.

### `link_artifacts`
Connect brain rows into the graph.
- Input: **`from_id`**, **`relation`** (`evidence`|`intel`|`hunts`|`detections`),
  **`to_ids`** (array).
- Writes: sets the relation property on `from_id` (via brain `Link`).

### `store_hunt`
Conclude the hunt (Conclude step).
- Input: **`outcome`** (`Confirmed`|`Refuted`|`Inconclusive`), **`findings`**
  (array of claims, each `{text, evidence[], hypothesis, confidence}`),
  `hypothesis`, `hypotheses` (array of claims), `next_steps` (array).
- Effect: lints the page (R-0004-11); `CloseHunt` sets status + summary +
  `ended_at`; `WriteHuntPage` persists the narrative. Returns an outcome-aware
  directive (R-0004-12).

### Flow

```
recall ─► queue_hunt ─► open_hunt ─► record_finding* ─► record_intel* ─► store_hunt ─► (Automate: SPEC-0006) ─► link_artifacts
   │           │            │  ▲                                              │
 read        backlog      hunts │──────────── rc.HuntID active ──────────────┘
```

## 5. Acceptance criteria

- **A-0004-01** (R-0004-03) `recall.ReadOnly()` is `true`; every other hunting
  tool's `ReadOnly()` is `false`.
- **A-0004-02** (R-0004-06) `open_hunt` with a `backlog_id` leaves the backlog
  row at `status=Opened` with its `hunt` relation set to the new hunt ID.
- **A-0004-03** (R-0004-07) `record_finding` returns a non-empty ID and the
  evidence row's `hunt` relation equals the active hunt.
- **A-0004-04** (R-0004-11) `store_hunt` returns an error result when a
  declarative finding cites no evidence; the same page with that claim marked
  `hypothesis: true` passes (mirrors `LintHuntPage` tests).
- **A-0004-05** (R-0004-10) A `store_hunt` outcome outside the three verdicts is
  rejected.
- **A-0004-06** (R-0004-02) `record_finding`/`store_hunt` with no active hunt
  return a clear error, not a silent write.

## 6. Non-goals

- **Detection authoring** — the Automate step's tools are
  [SPEC-0006](SPEC-0006-detection-authoring.md).
- **Evidence acquisition** — querying data lakes is
  [SPEC-0007](SPEC-0007-evidence-and-mcp.md); these tools only *record* what the
  evidence tools surface.

## 7. Open questions

- Should `store_hunt` optionally auto-create the `detections` row + `hunt →
  detection` link when the agent authors a rule in the same turn, rather than
  requiring a separate `link_artifacts` call?
- Should `record_finding` deduplicate identical pointers within a hunt?

## 8. References

- [SPEC-0002](SPEC-0002-brain-and-persistence.md) — the rows and relations written here.
- [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) §4 — the loop these tools implement.
- `internal/agent/prompt.go` — the prompt that sequences these tools.
