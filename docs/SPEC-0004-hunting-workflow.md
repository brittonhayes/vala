# SPEC-0004 · Hunting Workflow

> The nine tools that drive the loop: recall, queue, open, validate data, record
> findings and intel, link, conclude, and update coverage — each writing the
> brain through one session context.

| Field | Value |
|---|---|
| **ID** | SPEC-0004 |
| **Status** | Stable |
| **Updated** | 2026-06-10 |
| **Source of truth** | `internal/tools/{recall,queue_hunt,open_hunt,validate_data,record_finding,record_intel,link_artifacts,store_hunt,update_coverage}.go`, `internal/tools/runcontext.go` |
| **Depends on** | SPEC-0001, SPEC-0002, SPEC-0003 |

## 1. Purpose & scope

This spec defines the hunting tools — the agent-facing primitives that move a
hunt through the eight-stage loop of
[SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) and record every stage in the
brain. It fixes each tool's name, input, read-only status, brain effect, and the
shared session state they write through.

It does **not** define the brain's data model (that is
[SPEC-0002](SPEC-0002-brain-and-persistence.md)), the detection-authoring tools
used in the convert stage (that is
[SPEC-0006](SPEC-0006-detection-authoring.md)), nor the coverage map and metrics
the feedback stage produces (that is
[SPEC-0012](SPEC-0012-coverage-and-feedback.md)).

## 2. Definitions

- **RunContext (`rc`)** — per-session state the hunting tools share: the active
  hunt ID, question, and hunt type, the brain client, the findings recorded so
  far, and the per-hunt stage accumulators (visibility gaps, data-plan-validated
  flag, coverage-updated flag). `SetHunt(huntID, question, huntType)` sets the
  active hunt and resets the per-hunt accumulators.
- **Active hunt** — the hunt `open_hunt` set as current; `validate_data`,
  `record_finding`, and `store_hunt` target it.
- **Finding ID** — the ID `record_finding` (or `validate_data`) returns, which a
  concluding claim must cite.

## 3. Requirements

### Session state

- **R-0004-01** The hunting tools MUST share one `RunContext`. `open_hunt` MUST
  set the active hunt on it (id, question, hunt type) and reset the per-hunt
  accumulators; `validate_data`, `record_finding`, `record_intel`, and
  `store_hunt` MUST act on that active hunt.
- **R-0004-02** `validate_data`, `record_finding`, and `store_hunt` MUST fail
  clearly if no hunt is active, rather than writing an orphaned row.

### Tool contracts

- **R-0004-03** `recall` MUST be the only read-only hunting tool; it MUST read
  the brain back and MUST NOT write. Every other hunting tool mutates the brain
  and MUST be permission-gated.
- **R-0004-04** `recall` MUST accept a free-text `query` (empty lists recent), an
  optional `scope` over the brain stores
  (`all`|`hunts`|`intel`|`detections`|`backlog`|`coverage`), and an optional
  `limit` (default 5), returning compact per-store summaries.
- **R-0004-05** `queue_hunt` MUST write a backlog row in state `Queued` from a
  `trigger` + `hypothesis` (with optional behavior, data_source, priority,
  mitre) and return its ID.
- **R-0004-06** `open_hunt` MUST open a hunts row (state `Open`), record the
  hunt type (`hunt_type`, default `hypothesis`), set it as the active hunt, and
  return its ID. Given a `backlog_id`, it MUST mark that backlog item `Opened`
  and link it to the new hunt.
- **R-0004-13** `validate_data` MUST be the plan-&-validate-data stage (stage 3):
  given the data `sources` the hypothesis needs and a `validated` flag, on a
  pass it MUST record a `data_plan` evidence finding and mark the run's data plan
  validated; on a failure (`validated:false` or a `gap` set) it MUST record a
  `visibility_gap` evidence finding linked to the hunt and append it to the run's
  gaps. A failed check MUST NOT be a silent skip.
- **R-0004-07** `record_finding` MUST append an immutable evidence row to the
  active hunt and MUST return an ID for the agent to cite. It MUST require a
  `claim`, a `source` (`query` | `url` | `file_hash` | `log_ref`), and a
  `pointer`.
- **R-0004-08** `record_intel` MUST write an intel row (`indicator` | `ttp` |
  `actor` | `narrative`) and, when a hunt is active, link it to that hunt.
- **R-0004-09** `link_artifacts` MUST set a relation (`evidence` | `intel` |
  `hunts` | `detections`) on a source row to one or more target rows.
- **R-0004-10** `store_hunt` MUST close the active hunt with exactly one outcome
  (`Confirmed` | `Refuted` | `Inconclusive`) **and** exactly one
  `detection_tier` (`tier1_automated` … `tier5_none_documented`), accept an
  optional `tier_rationale`, write the narrative page, record the tier via
  `CloseHunt`, and enforce the full PEAK lint below.
- **R-0004-14** `update_coverage` MUST be the feedback stage (stage 8): given a
  `technique` and a coverage `status` (`Covered` | `Thin` | `Uncovered`), with
  optional `tactic`, `fidelity`, and `detections`, it MUST upsert a coverage row
  keyed by technique (via `UpsertCoverage`) and mark the run's coverage updated.

### The lint (PEAK invariants)

- **R-0004-11** `store_hunt` MUST run `LintHunt`, which layers the PEAK
  invariants on top of the citation linter (`LintHuntPage`) and rejects the hunt
  unless all hold:
  - **R-0004-11a** *citation discipline* — every declarative (non-hypothesis)
    finding MUST cite a recorded evidence ID; a claim is exempt only if
    explicitly marked a hypothesis (the `LintHuntPage` rule, unchanged).
  - **R-0004-11b** *validate before query* — if any `query`-kind evidence was
    recorded, the run MUST have a validated data plan OR at least one recorded
    visibility gap. A recorded gap satisfies the stage; a query with neither is
    rejected.
  - **R-0004-11c** *tier decision present & justified* — `detection_tier` MUST be
    non-empty and MUST carry a non-empty `tier_rationale`; a `tier5_none_documented`
    no-build especially MUST be justified.
  - **R-0004-11d** *feedback complete* — the run MUST have updated coverage
    (`update_coverage` was called) OR recorded at least one `next_steps` entry.
- **R-0004-12** `store_hunt` MUST return tier-aware conversion guidance keyed to
  the chosen `detection_tier`: tiers 1–2 steer the agent into authoring +
  linking a Sigma rule; tier 3 into queuing a recurring hunt (`queue_hunt`); tier
  4 into writing a playbook; tier 5 into recording the justified no-build (and a
  forensic-readiness follow-up when a visibility gap blocked the hunt). It MUST
  also remind the agent to feed back (`update_coverage`, `queue_hunt`).

## 4. Behavior & interfaces

Inputs below list `required` fields in **bold**. All tools except `recall` are
non-read-only (permission-gated).

### `recall` *(read-only)*
Search the brain before opening work, so hunts compound and settled ground is
not re-hunted.
- Input: **`query`** (string; empty = recent),
  `scope` (`all`|`hunts`|`intel`|`detections`|`backlog`|`coverage`),
  `limit` (int, default 5).
- Reads: hunts, intel, detections, backlog, coverage. Returns compact summaries
  (hunt: question/status/behavior; intel: kind/value/confidence; detection:
  title/status/path; backlog: hypothesis/status/priority; coverage:
  technique/status/fidelity). Scope `coverage` surfaces thin/uncovered ATT&CK
  techniques when scoping the next hunt.

### `queue_hunt`
Park a trigger as a prioritized backlog hypothesis (scope stage, deferred).
- Input: **`trigger`**, **`hypothesis`**, `behavior`, `data_source`,
  `priority` (`low`|`medium`|`high`), `mitre`.
- Writes: backlog row, `status=Queued`. Returns backlog ID.

### `open_hunt`
Open a hunt and make it active (stage 2: form hypothesis).
- Input: **`question`**, `hypothesis`, `behavior`, `data_source`, `mitre`,
  `hunt_type` (`hypothesis`|`baseline`|`model_assisted`, default `hypothesis`),
  `backlog_id`.
- Writes: hunts row, `status=Open`, `hunt_type`; calls
  `rc.SetHunt(id, question, hunt_type)`. With `backlog_id`: sets that backlog
  item `Opened` and links it. Returns hunt ID.

### `validate_data`
Validate the telemetry the hypothesis needs before querying (stage 3).
- Input: **`sources`** (array), `time_window`, `completeness`, `retention`,
  **`validated`** (bool), `gap` (string).
- Writes, linked to the active hunt:
  - `validated:true` (and no `gap`) → a `data_plan` evidence finding;
    marks `rc.dataPlanValidated`.
  - `validated:false` **or** `gap` set → a `visibility_gap` evidence finding;
    appends to `rc.gaps`. Never a silent skip; the message steers the agent to
    pivot or close `tier5_none_documented` with a forensic-readiness follow-up.

### `record_finding`
Record an immutable pointer that backs a claim (stages 4–5).
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
Conclude the hunt (stage 6: document & decide).
- Input: **`outcome`** (`Confirmed`|`Refuted`|`Inconclusive`),
  **`findings`** (array of claims, each `{text, evidence[], hypothesis, confidence}`),
  **`detection_tier`** (`tier1_automated`|`tier2_triage`|`tier3_recurring_hunt`|`tier4_playbook`|`tier5_none_documented`),
  `tier_rationale`, `hypothesis`, `hypotheses` (array of claims),
  `next_steps` (array).
- Effect: builds the page from the run's evidence, hunt type, gaps, data-plan
  and coverage flags; runs `LintHunt` (R-0004-11); `CloseHunt` sets status +
  summary + `ended_at` + tier + rationale; `WriteHuntPage` persists the
  narrative. Returns tier-aware conversion guidance (R-0004-12).

### `update_coverage`
Record the technique's coverage state (stage 8: feed back).
- Input: **`technique`** (ATT&CK ID), `tactic`,
  **`status`** (`Covered`|`Thin`|`Uncovered`),
  `fidelity` (`high`|`medium`|`low`|`none`), `detections`.
- Writes: upserts a coverage row keyed by technique (`UpsertCoverage`); marks
  `rc.coverageUpdated`. Returns the coverage row ID. See
  [SPEC-0012](SPEC-0012-coverage-and-feedback.md).

### Flow

```
recall ─► queue_hunt ─► open_hunt ─► validate_data ─► record_finding* ─► record_intel*
  (1)         (1)          (2)            (3)                (4–5)
   │           │            │  ▲                                  │
 read        backlog      hunts │────────── rc.HuntID active ─────┤
                                                                  ▼
        update_coverage ◄── (convert: SPEC-0006) ◄── store_hunt ──┘
             (8)              link_artifacts            (6–7)
```

## 5. Acceptance criteria

- **A-0004-01** (R-0004-03) `recall.ReadOnly()` is `true`; every other hunting
  tool's `ReadOnly()` is `false`.
- **A-0004-02** (R-0004-06) `open_hunt` with a `backlog_id` leaves the backlog
  row at `status=Opened` with its `hunt` relation set to the new hunt ID
  (`internal/tools/hunt_test.go` `TestOpenHuntConsumesBacklog`).
- **A-0004-03** (R-0004-07) `record_finding` returns a non-empty ID and the
  evidence row's `hunt` relation equals the active hunt
  (`TestRecordFindingTool`).
- **A-0004-04** (R-0004-11a) `store_hunt` returns an error result when a
  declarative finding cites no evidence; the same page with that claim marked
  `hypothesis: true` passes (`TestStoreHuntRejectsUnbackedFinding`,
  `internal/brain/hunt_test.go` `TestLintHuntPage*`).
- **A-0004-05** (R-0004-10) A `store_hunt` outcome outside the three verdicts is
  rejected, and `store_hunt` with no `detection_tier` is rejected
  (`internal/tools/hunt_test.go` `TestStoreHuntRejectsMissingTier`).
- **A-0004-06** (R-0004-02) `validate_data`/`record_finding`/`store_hunt` with no
  active hunt return a clear error, not a silent write.
- **A-0004-07** (R-0004-13) `validate_data` with `validated:true` records a
  `data_plan` evidence row and marks the data plan validated; with
  `validated:false` (or a `gap`) it records a `visibility_gap` row and appends a
  gap (`TestValidateDataRecordsPlan`, `TestValidateDataRecordsGapOnFailure`).
- **A-0004-08** (R-0004-11b) `store_hunt` rejects a hunt that recorded `query`-kind
  evidence without a validated data plan and with no recorded gap; a recorded
  gap satisfies it (`internal/tools/hunt_test.go`
  `TestStoreHuntRejectsQueryBeforeValidation`; `internal/brain/hunt_test.go`
  `TestLintHuntRejectsQueryBeforeValidation`,
  `TestLintHuntAcceptsRecordedGapInsteadOfPlan`).
- **A-0004-09** (R-0004-11c) `store_hunt` rejects a missing tier decision and an
  unjustified one — a `tier5_none_documented` without a rationale most of all
  (`internal/brain/hunt_test.go` `TestLintHuntRequiresTierDecision`,
  `TestLintHuntRequiresJustifiedNoBuild`).
- **A-0004-10** (R-0004-11d) `store_hunt` rejects a hunt that neither updated
  coverage nor recorded a next step (`internal/brain/hunt_test.go`
  `TestLintHuntRequiresFeedback`).
- **A-0004-11** (R-0004-14) `update_coverage` upserts a coverage row keyed by
  technique and marks the run's coverage updated
  (`internal/brain/coverage_test.go` `TestUpsertCoverageCreatesThenUpdates`).
- **A-0004-12** (R-0004-05..14) The full eight-stage loop runs end to end through
  these tools against `Mem` (`internal/tools/peak_e2e_test.go`
  `TestPEAKLoopEndToEnd`).

## 6. Non-goals

- **Detection authoring** — the convert-stage tools (tiers 1–2) are
  [SPEC-0006](SPEC-0006-detection-authoring.md).
- **Evidence acquisition** — querying data lakes is
  [SPEC-0007](SPEC-0007-evidence-and-mcp.md); these tools only *record* what the
  evidence tools surface.
- **Coverage metrics** — what coverage means and the suggested metric set live in
  [SPEC-0012](SPEC-0012-coverage-and-feedback.md); `update_coverage` only writes
  the row.

## 7. Open questions

- Should `store_hunt` optionally auto-create the `detections` row + `hunt →
  detection` link when the agent authors a rule in the same turn, rather than
  requiring a separate `link_artifacts` call?
- Should `record_finding` deduplicate identical pointers within a hunt?
- Should `update_coverage` derive `fidelity` from the chosen detection tier
  automatically rather than asking the agent to map it?

## 8. References

- [SPEC-0002](SPEC-0002-brain-and-persistence.md) — the rows and relations written here.
- [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) §4 — the loop these tools implement.
- [SPEC-0012](SPEC-0012-coverage-and-feedback.md) — the coverage store and feedback metrics.
- `internal/agent/prompt.go` — the prompt that sequences these tools.
