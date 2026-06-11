# SPEC-0002 · Brain & Persistence

> vala's memory is a seven-table graph — backlog, hunts, evidence, intel,
> detections, memory, coverage — backed by an in-memory store, a JSON file, or a
> Notion workspace. A Notion workspace makes memory multiplayer: one team, one
> brain.

| Field | Value |
|---|---|
| **ID** | SPEC-0002 |
| **Status** | Stable |
| **Updated** | 2026-06-10 |
| **Source of truth** | `internal/brain/` |
| **Depends on** | SPEC-0001 |

## 1. Purpose & scope

This spec defines vala's persistence layer — "the brain": the seven logical
tables, the fields and relations on each, the interchangeable storage backends,
and how a Notion brain is provisioned. It covers the data model and the storage
contract.

It does **not** cover the tools that write the brain (that is
[SPEC-0004](SPEC-0004-hunting-workflow.md)) nor the configuration that points
vala at a Notion workspace (that is [SPEC-0009](SPEC-0009-configuration.md)).

## 2. Definitions

- **Store / table** — one of the seven logical databases, named by a `DB*`
  constant: `evidence`, `hunts`, `intel`, `detections`, `backlog`, `memory`,
  `coverage`.
- **Row** — one artifact in a store: `{ID, DB, Props}` where `Props` is a flat
  `map[string]any` of property name → value.
- **Relation** — a property whose value is a list of row IDs, forming a graph
  edge between stores.
- **Backend** — the concrete storage implementing the `Notion` interface: `Mem`
  (in-memory), `File` (a JSON file on disk), or `NTN` (a real Notion workspace
  via the `ntn` CLI / API).
- **Data-source ID** — Notion's identifier for the queryable schema behind a
  database; the brain reads and writes against data-source IDs, not database IDs.

## 3. Requirements

### Data model

- **R-0002-01** The brain MUST consist of exactly seven logical stores: evidence,
  hunts, intel, detections, backlog, memory, coverage. The set of stores is the
  entire persistence surface.
- **R-0002-02** `brain.Schema()` MUST be the single source of truth for the
  shape of every store: each store's title column, scalar properties, relation
  properties, and any status options. A property a writer emits MUST appear in
  `Schema()`, or it will be silently dropped on write to Notion.
- **R-0002-03** Each store MUST have exactly one `title`-typed property (the
  display column).
- **R-0002-04** Every row MUST carry a creation/collection timestamp in RFC 3339
  format (`started_at`, `created_at`, or `collected_at` per store).
- **R-0002-05** Relations MUST be written only when non-empty; an empty relation
  MUST NOT appear in a row's properties.

### State machines

- **R-0002-06** A **hunt** MUST occupy exactly one `status`: it is born `Open`
  and closes into one terminal state — `Confirmed`, `Refuted`, or
  `Inconclusive`. There is no transition out of a terminal state.
- **R-0002-07** A **backlog item** MUST progress linearly `Queued → Opened →
  Done`. Moving to `Opened` or `Done` SHOULD set the item's `hunt` relation to
  the hunt it became.
- **R-0002-15** A **coverage** row MUST occupy exactly one `status` —
  `Covered`, `Thin`, or `Uncovered` — and there MUST be at most one row per
  ATT&CK technique: `UpsertCoverage` keys on `technique`, updating the existing
  row rather than duplicating it (see [SPEC-0012](SPEC-0012-coverage-and-feedback.md)).

### Backends

- **R-0002-08** Both backends MUST implement one `Notion` interface so the brain
  client is backend-agnostic. The four methods are `CreateRow`, `UpdateRow`,
  `CreatePage`, `Query`.
- **R-0002-09** With no brain configured, vala MUST run in an ephemeral in-memory
  brain (`Mem`) — fully functional, forgotten on exit. A configured `brain_file`
  selects the durable `File` backend; configured Notion IDs select `NTN`. Nothing
  else changes for callers: every backend implements the same `Notion` interface.
- **R-0002-10** For the `NTN` backend, the brain client MUST translate logical
  store names to the configured Notion data-source IDs; for `Mem` it MUST use
  the logical names directly.
- **R-0002-11** A `status` column MUST be provisioned with exactly its allowed
  values up front (Notion does not auto-create status options on write, unlike
  `select`).

### Provisioning

- **R-0002-12** Provisioning MUST create a single Notion database titled "Vala
  Brain" under the home page, holding one **data source** per store (the same
  seven stores). The first store becomes the database's initial data source; the
  rest are added as data sources under that database. It MUST run in two passes:
  scalar properties first, then relation properties once every target data
  source exists.
- **R-0002-13** Provisioning MUST verify the operator is authenticated to Notion
  (`ntn whoami`) before creating anything.
- **R-0002-14** A configured brain MUST be verifiable and repairable in place:
  `NTN.Verify` MUST report which stores are missing and whether the parent
  database resolves. When data sources are missing, `NTN.AddMissing` MUST
  re-create only those store(s) under the existing "Vala Brain" database; when
  the parent database itself is gone, provisioning MUST re-create a fresh single
  database. Verification and repair MUST NOT duplicate stores that already
  resolve (the repair flow runs from `vala setup`; see
  [SPEC-0010](SPEC-0010-cli.md)).

## 4. Behavior & interfaces

### The `Notion` interface

```go
type Notion interface {
    CreateRow(ctx, db string, props map[string]any) (id string, err error)
    UpdateRow(ctx, id string, props map[string]any) error
    CreatePage(ctx, title, markdown string) (id, url string, err error)
    Query(ctx, db, query string, limit int) ([]Row, error)
}

type Row struct {
    ID    string
    DB    string
    Props map[string]any
}
```

`brain.New(n Notion) *Client` wraps a backend. If `n` is `*NTN`, it installs a
name-mapper that turns each logical `DB*` name into the configured data-source
ID; otherwise logical names pass through unchanged (R-0002-10).

### The seven stores

Property types are Notion types: `title`, `rich_text`, `select`, `status`,
`date`. Relations name their target store.

**evidence** — immutable finding pointers (title: `claim`)

| Property | Type | Notes |
|---|---|---|
| `claim` | title | the claim the pointer backs |
| `kind` | select | `query` \| `url` \| `file_hash` \| `log_ref` \| `data_plan` \| `visibility_gap` |
| `pointer` | rich_text | the immutable pointer (query ID, URL, hash, log ref, data-plan summary, or affected sources) |
| `confidence` | select | `confirmed` \| `probable` \| `hypothesis` |
| `collected_at` | date | RFC 3339 |
| `hunt` → hunts | relation | the hunt this finding backs |

`kind` is a Notion `select`, so its options auto-create on write; `data_plan`
(a validated telemetry plan) and `visibility_gap` (a failed telemetry check)
are the validate-data stage's two evidence kinds (see
[SPEC-0004](SPEC-0004-hunting-workflow.md), [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) §4).

**hunts** — hypothesis-driven hunts (title: `hunt_id`)

| Property | Type | Notes |
|---|---|---|
| `hunt_id` | title | set to the question text |
| `question` | rich_text | |
| `hypothesis` | rich_text | |
| `status` | status | `Open` → {`Confirmed`,`Refuted`,`Inconclusive`} |
| `mitre` | rich_text | ATT&CK technique(s) |
| `behavior` | rich_text | ABLE behavior (optional) |
| `data_source` | rich_text | ABLE location (optional) |
| `hunt_type` | select | `hypothesis` \| `baseline` \| `model_assisted`; set on open |
| `detection_tier` | select | the detection-output tier; set on close |
| `tier_rationale` | rich_text | why that tier; set on close |
| `findings` | rich_text | summary, written on close |
| `started_at` | date | set on open |
| `ended_at` | date | set on close |

**intel** — reusable threat intelligence (title: `intel_id`)

| Property | Type | Notes |
|---|---|---|
| `intel_id` | title | set to the value |
| `kind` | select | `indicator` \| `ttp` \| `actor` \| `narrative` |
| `value` | rich_text | the IOC / technique / actor / text |
| `mitre` | rich_text | |
| `confidence` | select | |
| `source` | rich_text | |
| `description` | rich_text | |
| `created_at` | date | |
| `hunts` → hunts | relation | hunts that surfaced this intel |
| `detections` → detections | relation | detections this intel informs |

**detections** — graph node for a Sigma rule (title: `detection_id`). The rule
YAML lives on disk; this row makes hunt/intel → detection relations first-class.

| Property | Type | Notes |
|---|---|---|
| `detection_id` | title | the Sigma rule id/title |
| `title` | rich_text | |
| `path` | rich_text | file path on disk |
| `status` | select | the Sigma rule status |
| `mitre` | rich_text | |
| `level` | select | the Sigma severity level |
| `intel` → intel | relation | intel this detection implements |
| `hunts` → hunts | relation | hunts this detection covers |

**backlog** — queued, prioritized hypotheses (title: `backlog_id`)

| Property | Type | Notes |
|---|---|---|
| `backlog_id` | title | set to the trigger text |
| `trigger` | rich_text | intel, hunch, CVE, past incident |
| `hypothesis` | rich_text | |
| `status` | status | `Queued` → `Opened` → `Done` |
| `behavior` | rich_text | ABLE behavior (optional) |
| `data_source` | rich_text | ABLE location (optional) |
| `priority` | select | optional |
| `mitre` | rich_text | optional |
| `created_at` | date | |
| `hunt` → hunts | relation | the hunt it opened into |

**memory** — durable, shareable operator facts about the environment (title:
`memory_id`). Because memory lives in the brain, a team on one Notion workspace
shares each other's memories; each row records who learned it.

| Property | Type | Notes |
|---|---|---|
| `memory_id` | title | set to the fact text |
| `fact` | rich_text | the durable environment fact |
| `author` | rich_text | the operator who recorded it |
| `created_at` | date | RFC 3339 |
| `hunt` → hunts | relation | the hunt that taught it (optional) |

**coverage** — the cross-hunt detection-coverage map, one row per ATT&CK
technique (title: `technique`). The feedback stage upserts it (keyed by
technique) as hunts conclude; scoping reads it to aim the next hypothesis at the
weakest spots. See [SPEC-0012](SPEC-0012-coverage-and-feedback.md).

| Property | Type | Notes |
|---|---|---|
| `technique` | title | ATT&CK technique ID, e.g. `attack.t1562.001` |
| `tactic` | rich_text | the ATT&CK tactic (optional) |
| `status` | status | `Covered` \| `Thin` \| `Uncovered` |
| `fidelity` | select | `high` \| `medium` \| `low` \| `none` |
| `detections` | rich_text | summary of the detections that cover it |
| `updated_at` | date | RFC 3339 |
| `hunts` → hunts | relation | hunts that touched this technique |
| `detections` → detections | relation | detections covering it |

### The graph

```
Backlog ─►(opened as) Hunts ─►(produced) Detections
   ▲                    │ ▲                    ▲
   │                    ▼ │                    │
  Intel ───(surfaced / informs)───────────────┘
            Evidence ──(backs)── Hunts
             Memory ──(learned during)── Hunts
           Coverage ──(touched by / covered by)── Hunts, Detections
```

### Writer functions (the brain `Client`)

These are the only writers; each emits the exact property names above.

| Function | Effect |
|---|---|
| `OpenHunt(h Hunt) → id` | creates a hunts row, `status=Open`, `started_at=now` |
| `RecordFinding(huntID, e Evidence) → id` | creates an evidence row linked to the hunt |
| `CloseHunt(huntID, status, findings, detectionTier, tierRationale)` | sets `status`, `findings`, `ended_at=now`, and the detection-tier decision |
| `WriteHuntPage(title, p HuntPage) → url` | renders the narrative markdown page |
| `RecordIntel(i Intel) → id` | creates an intel row; inline `hunts`/`detections` relations |
| `RecordDetection(d DetectionRef) → id` | creates a detections row; inline `intel`/`hunts` relations |
| `QueueHunt(b BacklogItem) → id` | creates a backlog row, `status=Queued` |
| `SetBacklogStatus(id, status, huntID)` | transitions status, sets `hunt` relation when given |
| `Link(rowID, relation, targetIDs…)` | the single relation primitive; no-op on empty targets |
| `Remember(m Memory) → id` | creates a memory row stamped with `author` (and an optional `hunt` relation) |
| `Memories(query, limit) → []Memory` | typed read-back of shared memory, text-extracted across backends |
| `UpsertCoverage(cov Coverage) → id` | upserts a coverage row keyed by `technique`: updates the matching row (via `findCoverage`, exact-technique match) or creates one; stamps `updated_at` |
| `Recall(db, query, limit) → []Row` | free-text read-back, including the `coverage` store (see below) |

### Recall semantics

`Recall(db, query, limit)` returns up to `limit` rows (default 5 when ≤ 0) from
one store, matching `query` as a case-insensitive substring over the row's
serialized properties; an empty query lists recent rows. `Mem` filters
in-memory; `NTN` queries the data source and filters client-side.

### Backends

- **Mem** (`NewMem`) — synthetic IDs (`{db}_{seq}`), pages at `mem://{id}`,
  mutex-guarded, no network. Used in tests and unconfigured runs. Exposes
  `RowsIn(db)` for assertions.
- **File** (`NewFile`) — the same synthetic-ID, substring-query semantics as
  `Mem`, persisted to a single JSON file written atomically (temp + rename) on
  every mutation and reloaded on open, so the ID sequence and rows survive across
  sessions. Narrative pages are written as readable `.md` files in a `pages`
  directory beside the JSON. Selected by the `brain_file` config key (chosen as
  the on-disk option in the `vala setup` wizard); a durable brain with no
  external account.
- **NTN** — shells the operator's authenticated `ntn` CLI / Notion API. Holds a
  `DBIDs` struct (the parent `database` ID + one data-source ID per store + a
  narrative parent page ID), lazily caches each data source's property schema,
  and coerces flat props into Notion typed property objects. Requires
  `ntn login`.

### Provisioning (from `vala setup`)

1. `Whoami` — verify authentication (R-0002-13).
2. Create the home page; the narrative hunt pages are written directly beneath
   it (`page_parent` is that home page — there is no separate "Vala Hunt Pages"
   wrapper page).
3. Create **one** database titled "Vala Brain" under the home page from the
   first `DBSpec` in `Schema()` → returns `(databaseID, dsID)`. For every
   remaining `DBSpec`: `POST /v1/data_sources` with a `database_id` parent adds
   that store as another data source on the same database. Status columns are
   seeded with their allowed values (R-0002-11).
4. Second pass: `AddRelations(dsID, …)` wires each relation to its target's
   data-source ID (R-0002-12).
5. `DBIDsFromMap(...)` assembles the `DBIDs` (the `database` ID, each store's
   data-source ID, and `page_parent`), persisted to `./.vala.json` under
   `notion` (see [SPEC-0009](SPEC-0009-configuration.md)).

Repair: if a configured brain is missing a data source, `NTN.AddMissing` adds
the missing store(s) to the existing "Vala Brain" database; if the database
itself is gone, a fresh single database is provisioned (R-0002-14).

## 5. Acceptance criteria

- **A-0002-01** (R-0002-01) `brain.Schema()` returns exactly seven `DBSpec`s
  named evidence, hunts, intel, detections, backlog, memory, coverage.
- **A-0002-02** (R-0002-02, R-0002-03) Every property emitted by a writer in
  `hunt.go`/`intel.go`/`backlog.go`/`coverage.go` appears in the matching
  `DBSpec.Props` or `Relations`; each `DBSpec` has exactly one `title` prop.
- **A-0002-03** (R-0002-06) `CloseHunt` only ever writes a status of `Confirmed`,
  `Refuted`, or `Inconclusive`, plus the chosen detection tier and rationale;
  `OpenHunt` writes `Open` and the hunt type.
- **A-0002-04** (R-0002-07) `SetBacklogStatus(id, "Opened", huntID)` sets
  `status=Opened` and `hunt=[huntID]`.
- **A-0002-05** (R-0002-09) With an empty `notion` config, `brainStore()` yields
  a `Mem` store and the full loop runs end to end (existing brain tests pass
  against `Mem`).
- **A-0002-06** (R-0002-05) `setRelation` / `Link` write no relation key when
  the target list is empty.
- **A-0002-07** (R-0002-14) `NTN.Verify` against a valid config reports no
  missing stores and a resolving database; against a config with a missing
  data source it flags that store, and `NTN.AddMissing` re-creates only it under
  the existing "Vala Brain" database without duplicating the others.
- **A-0002-08** (R-0002-15) `UpsertCoverage` for a technique creates a row the
  first time and updates that same row on a second call with the same technique
  (`internal/brain/coverage_test.go` `TestUpsertCoverageCreatesThenUpdates`).

## 6. Non-goals

- **No query language.** `Recall` is substring search, not structured query;
  richer querying is the operator's job in Notion directly.
- **No schema migration.** Changing `Schema()` against an already-provisioned
  workspace is not auto-migrated; the operator re-provisions.
- **No backend beyond Mem, File, and NTN.** Other stores are out of scope.

## 7. Open questions

- Should `Recall` rank or score results rather than return first-N substring
  matches?
- Should terminal hunts ever reopen (e.g. an `Inconclusive` hunt that gets new
  evidence), and if so what status models that?

## 8. References

- [SPEC-0004](SPEC-0004-hunting-workflow.md) — the tools that drive these writers.
- [SPEC-0009](SPEC-0009-configuration.md) — the `notion` config block and `DBIDs`.
- `internal/brain/provision.go` — `Schema()`, the canonical shape.
