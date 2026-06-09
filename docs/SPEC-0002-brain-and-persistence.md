# SPEC-0002 · Brain & Persistence

> vala's memory is a five-table graph — backlog, hunts, evidence, intel,
> detections — backed by either an in-memory store or a Notion workspace.

| Field | Value |
|---|---|
| **ID** | SPEC-0002 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `internal/brain/` |
| **Depends on** | SPEC-0001 |

## 1. Purpose & scope

This spec defines vala's persistence layer — "the brain": the five logical
tables, the fields and relations on each, the two interchangeable storage
backends, and how a Notion brain is provisioned. It covers the data model and
the storage contract.

It does **not** cover the tools that write the brain (that is
[SPEC-0004](SPEC-0004-hunting-workflow.md)) nor the configuration that points
vala at a Notion workspace (that is [SPEC-0009](SPEC-0009-configuration.md)).

## 2. Definitions

- **Store / table** — one of the five logical databases, named by a `DB*`
  constant: `evidence`, `hunts`, `intel`, `detections`, `backlog`.
- **Row** — one artifact in a store: `{ID, DB, Props}` where `Props` is a flat
  `map[string]any` of property name → value.
- **Relation** — a property whose value is a list of row IDs, forming a graph
  edge between stores.
- **Backend** — the concrete storage implementing the `Notion` interface: `Mem`
  (in-memory) or `NTN` (a real Notion workspace via the `ntn` CLI / API).
- **Data-source ID** — Notion's identifier for the queryable schema behind a
  database; the brain reads and writes against data-source IDs, not database IDs.

## 3. Requirements

### Data model

- **R-0002-01** The brain MUST consist of exactly five logical stores: evidence,
  hunts, intel, detections, backlog. The set of stores is the entire persistence
  surface.
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

### Backends

- **R-0002-08** Both backends MUST implement one `Notion` interface so the brain
  client is backend-agnostic. The four methods are `CreateRow`, `UpdateRow`,
  `CreatePage`, `Query`.
- **R-0002-09** With no Notion configured, vala MUST run in an ephemeral
  in-memory brain (`Mem`) — fully functional, forgotten on exit. Configuration
  selects `NTN`; nothing else changes for callers.
- **R-0002-10** For the `NTN` backend, the brain client MUST translate logical
  store names to the configured Notion data-source IDs; for `Mem` it MUST use
  the logical names directly.
- **R-0002-11** A `status` column MUST be provisioned with exactly its allowed
  values up front (Notion does not auto-create status options on write, unlike
  `select`).

### Provisioning

- **R-0002-12** `vala init` MUST provision all five databases under a parent
  page, in two passes: scalar properties first, then relation properties once
  every target data source exists.
- **R-0002-13** Provisioning MUST verify the operator is authenticated to Notion
  (`ntn whoami`) before creating anything.
- **R-0002-14** Provisioning MUST be idempotent: re-running against an existing,
  valid configuration verifies and reuses it rather than duplicating databases
  (override with `--force`; see [SPEC-0010](SPEC-0010-cli.md)).

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

### The five stores

Property types are Notion types: `title`, `rich_text`, `select`, `status`,
`date`. Relations name their target store.

**evidence** — immutable finding pointers (title: `claim`)

| Property | Type | Notes |
|---|---|---|
| `claim` | title | the claim the pointer backs |
| `kind` | select | `query` \| `url` \| `file_hash` \| `log_ref` |
| `pointer` | rich_text | the immutable pointer (query ID, URL, hash, log ref) |
| `confidence` | select | `confirmed` \| `probable` \| `hypothesis` |
| `collected_at` | date | RFC 3339 |
| `hunt` → hunts | relation | the hunt this finding backs |

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

### The graph

```
Backlog ─►(opened as) Hunts ─►(produced) Detections
   ▲                    │ ▲                    ▲
   │                    ▼ │                    │
  Intel ───(surfaced / informs)───────────────┘
            Evidence ──(backs)── Hunts
```

### Writer functions (the brain `Client`)

These are the only writers; each emits the exact property names above.

| Function | Effect |
|---|---|
| `OpenHunt(h Hunt) → id` | creates a hunts row, `status=Open`, `started_at=now` |
| `RecordFinding(huntID, e Evidence) → id` | creates an evidence row linked to the hunt |
| `CloseHunt(huntID, status, findings)` | sets `status`, `findings`, `ended_at=now` |
| `WriteHuntPage(title, p HuntPage) → url` | renders the narrative markdown page |
| `RecordIntel(i Intel) → id` | creates an intel row; inline `hunts`/`detections` relations |
| `RecordDetection(d DetectionRef) → id` | creates a detections row; inline `intel`/`hunts` relations |
| `QueueHunt(b BacklogItem) → id` | creates a backlog row, `status=Queued` |
| `SetBacklogStatus(id, status, huntID)` | transitions status, sets `hunt` relation when given |
| `Link(rowID, relation, targetIDs…)` | the single relation primitive; no-op on empty targets |
| `Recall(db, query, limit) → []Row` | free-text read-back (see below) |

### Recall semantics

`Recall(db, query, limit)` returns up to `limit` rows (default 5 when ≤ 0) from
one store, matching `query` as a case-insensitive substring over the row's
serialized properties; an empty query lists recent rows. `Mem` filters
in-memory; `NTN` queries the data source and filters client-side.

### Backends

- **Mem** (`NewMem`) — synthetic IDs (`{db}_{seq}`), pages at `mem://{id}`,
  mutex-guarded, no network. Used in tests and unconfigured runs. Exposes
  `RowsIn(db)` for assertions.
- **NTN** — shells the operator's authenticated `ntn` CLI / Notion API. Holds a
  `DBIDs` struct (one data-source ID per store + a narrative parent page ID),
  lazily caches each data source's property schema, and coerces flat props into
  Notion typed property objects. Requires `ntn login`.

### Provisioning (`vala init`)

1. `Whoami` — verify authentication (R-0002-13).
2. For each `DBSpec` in `Schema()`: `CreateDatabase(parent, title, props,
   statusOptions)` → returns `(dbID, dsID)`; status columns are seeded with their
   allowed values (R-0002-11).
3. Create the narrative parent page (hunt pages are written beneath it).
4. Second pass: `AddRelations(dsID, …)` wires each relation to its target's
   data-source ID (R-0002-12).
5. `DBIDsFromMap(...)` assembles the `DBIDs`, persisted to `./.vala.json` under
   `notion` (see [SPEC-0009](SPEC-0009-configuration.md)).

## 5. Acceptance criteria

- **A-0002-01** (R-0002-01) `brain.Schema()` returns exactly five `DBSpec`s
  named evidence, hunts, intel, detections, backlog.
- **A-0002-02** (R-0002-02, R-0002-03) Every property emitted by a writer in
  `hunt.go`/`intel.go`/`backlog.go` appears in the matching `DBSpec.Props` or
  `Relations`; each `DBSpec` has exactly one `title` prop.
- **A-0002-03** (R-0002-06) `CloseHunt` only ever writes a status of `Confirmed`,
  `Refuted`, or `Inconclusive`; `OpenHunt` writes `Open`.
- **A-0002-04** (R-0002-07) `SetBacklogStatus(id, "Opened", huntID)` sets
  `status=Opened` and `hunt=[huntID]`.
- **A-0002-05** (R-0002-09) With an empty `notion` config, `brainStore()` yields
  a `Mem` store and the full loop runs end to end (existing brain tests pass
  against `Mem`).
- **A-0002-06** (R-0002-05) `setRelation` / `Link` write no relation key when
  the target list is empty.
- **A-0002-07** (R-0002-14) Running `vala init` twice against the same parent
  with valid IDs does not create duplicate databases.

## 6. Non-goals

- **No query language.** `Recall` is substring search, not structured query;
  richer querying is the operator's job in Notion directly.
- **No schema migration.** Changing `Schema()` against an already-provisioned
  workspace is not auto-migrated; the operator re-provisions.
- **No backend beyond Mem and NTN.** Other stores are out of scope.

## 7. Open questions

- Should `Recall` rank or score results rather than return first-N substring
  matches?
- Should terminal hunts ever reopen (e.g. an `Inconclusive` hunt that gets new
  evidence), and if so what status models that?

## 8. References

- [SPEC-0004](SPEC-0004-hunting-workflow.md) — the tools that drive these writers.
- [SPEC-0009](SPEC-0009-configuration.md) — the `notion` config block and `DBIDs`.
- `internal/brain/provision.go` — `Schema()`, the canonical shape.
