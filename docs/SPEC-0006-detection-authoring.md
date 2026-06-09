# SPEC-0006 · Detection Authoring

> The Automate step's toolkit: consult exemplars, edit single Sigma fields
> without disturbing comments, attach a runbook and tests, then validate and run.

| Field | Value |
|---|---|
| **ID** | SPEC-0006 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `internal/tools/{reference,detection,detection_edit,*detection*}.go`, `internal/sigma/edit.go`, `internal/reference/` |
| **Depends on** | SPEC-0001, SPEC-0003, SPEC-0005 |

## 1. Purpose & scope

This spec defines how vala authors a detection in the Automate step: the
reference exemplars the agent learns shape from, the surgical field-editing tools
that change one Sigma field at a time while preserving comments and key order,
the custom `runbook:` and `tests:` fields, and the validate/test tools that gate
"done."

It does **not** define the matching/validation engine itself (that is
[SPEC-0005](SPEC-0005-detection-engine.md)) nor the brain `detections` row (that
is [SPEC-0002](SPEC-0002-brain-and-persistence.md)).

## 2. Definitions

- **Field tool** — a tool that edits exactly one part of a Sigma rule
  (metadata, logsource, detection logic, a list field, the runbook, or tests).
- **Runbook** — the custom top-level `runbook:` map giving response guidance.
- **Reference exemplar** — an embedded gold-standard Sigma rule the agent
  consults before authoring.

## 3. Requirements

### Authoring discipline

- **R-0006-01** The agent SHOULD consult `reference_detection` for the closest
  exemplar before authoring, matching its shape: tight conditions, commented
  filters, populated `falsepositives`, an inline `runbook:`, and `tests:`.
- **R-0006-02** The agent SHOULD prefer the field tools over rewriting a rule
  with `write`: a field tool changes one field, preserves comments and key
  order, and re-validates in one step.
- **R-0006-03** Every authored rule SHOULD carry an inline `runbook:` (so it is
  respondable) and `tests:` with at least one should-match and one
  should-not-match case.
- **R-0006-04** After writing or editing a rule, the agent MUST validate it with
  `validate_detection` and fix every reported issue before considering the task
  done. It MUST NOT shell out to external validators.
- **R-0006-05** The agent MUST run `test_detection` and fix every failing case
  before finishing an authored rule.

### Field-edit contract

- **R-0006-06** A field tool MUST preserve YAML comments and key order outside
  the field it changes, operating on the YAML node tree rather than a
  decode/re-encode round trip.
- **R-0006-07** A field tool MUST operate on a single YAML document and MUST
  reject a multi-document file (to avoid silently editing the wrong rule).
- **R-0006-08** A field tool MUST re-validate the rule after the edit, surfacing
  any schema error it introduced.
- **R-0006-09** A field tool MUST create the block it targets if absent
  (e.g. `logsource`, `detection`, `runbook`, `tests`) rather than failing.

### Tools

- **R-0006-10** `reference_detection` (read-only) MUST list exemplars and return
  the full YAML of a named one.
- **R-0006-11** `validate_detection` and `test_detection` (read-only) MUST
  invoke the [SPEC-0005](SPEC-0005-detection-engine.md) engine over the given
  path/dir.
- **R-0006-12** The editing tools (`set_detection_meta`,
  `set_detection_logsource`, `edit_detection_logic`, `manage_detection_list`,
  `set_detection_runbook`, `manage_detection_tests`) are non-read-only and MUST
  be permission-gated.
- **R-0006-13** `set_detection_meta` MUST generate a UUID v4 for `id` when given
  the sentinel value `generate`.

### Runbook & tests shapes

- **R-0006-14** `runbook:` MUST be a top-level map whose keys are `triage`,
  `investigate`, `contain`, `escalate`, `references`, each an array of strings.
- **R-0006-15** `tests:` MUST be a top-level list of `{name, event, match}`
  cases as defined in [SPEC-0005](SPEC-0005-detection-engine.md) §R-0005-12.

## 4. Behavior & interfaces

Read-only tools are marked *(read-only)*; the rest are permission-gated. All
operate relative to the configured detections/working directory.

### Readers

- **`reference_detection`** *(read-only)* — `list` (bool) and/or `name`
  (string). Lists embedded exemplars (name, title, level, tags, description) or
  returns one's full YAML.
- **`validate_detection`** *(read-only)* — `path` and/or `dir`, `recursive`.
  Runs the schema check (SPEC-0005) over `*.yml`/`*.yaml`.
- **`test_detection`** *(read-only)* — **`path`**. Runs the rule's inline tests.

### Editors (write `.yml` under the working dir, re-validate after)

- **`set_detection_meta`** — **`path`**, plus any of `title`, `id`
  (`generate` → UUID v4), `status`, `description`, `author`, `date`, `level`.
- **`set_detection_logsource`** — **`path`**, plus any of `product`, `service`,
  `category`, `definition`.
- **`edit_detection_logic`** — **`path`**; set a named search identifier
  (`selection` + `fields` object), `remove` it, and/or set the `condition`.
- **`manage_detection_list`** — **`path`**, **`field`** (one of `references`,
  `falsepositives`, `tags`, `fields`), `add` and/or `remove` an item.
- **`set_detection_runbook`** — **`path`**, plus any of `triage`,
  `investigate`, `contain`, `escalate`, `references` (each an array of strings).
- **`manage_detection_tests`** — **`path`**, **`name`**, plus `event` (object),
  `match` (bool), or `remove` (bool) to drop a named case.

### Surgical editing (`internal/sigma`)

The `Editor` wraps a parsed `yaml.Node` tree. `Load` parses exactly one document
(rejecting multi-doc, R-0006-07); `Bytes` re-serializes with comments and order
intact. Top-level ops: `SetScalar`, `SetNode`, `Delete`, `Has`,
`EnsureMapping`, `AppendListItem`, `RemoveListItem`, `RemoveMapItem`. Mapping
ops: `SetInMapping`, `SetScalarInMapping`, `DeleteInMapping`. Multi-line scalars
use literal block style (`|`).

### Runbook field

```yaml
runbook:
  triage: [ "...first steps to size up the alert..." ]
  investigate: [ "...related events, scope, intent..." ]
  contain: [ "...stop or limit the activity..." ]
  escalate: [ "...when and to whom..." ]
  references: [ "https://..." ]
```

### Reference exemplars

Embedded under `internal/reference/sigma/` (AWS-focused, e.g. CloudTrail logging
disabled, Config recording disabled, GuardDuty disruption, IAM backdoor access
key, root account usage). `reference.List()` returns metadata; `reference.Get(name)`
returns full bytes. All exemplars validate and pass their inline tests (enforced
by tests; see [SPEC-0005](SPEC-0005-detection-engine.md) A-0005-06).

### A typical Automate sequence

```
reference_detection(name) → write/edit_detection_logic → set_detection_meta
  → set_detection_logsource → manage_detection_list(falsepositives, tags)
  → set_detection_runbook → manage_detection_tests(×2)
  → validate_detection → test_detection → (record + link_artifacts, SPEC-0004)
```

## 5. Acceptance criteria

- **A-0006-01** (R-0006-06) Editing one field with a field tool leaves all other
  lines — including comments — byte-identical except the changed field.
- **A-0006-02** (R-0006-07) A field tool on a multi-document file returns an
  error and writes nothing.
- **A-0006-03** (R-0006-08/09) `set_detection_runbook` on a rule with no
  `runbook:` creates the block and the result re-validates.
- **A-0006-04** (R-0006-13) `set_detection_meta` with `id: generate` produces a
  syntactically valid UUID v4.
- **A-0006-05** (R-0006-10/11) `reference_detection`, `validate_detection`, and
  `test_detection` report `ReadOnly() == true`; the editors report `false`.
- **A-0006-06** (R-0006-14/15) A rule authored via the tools carries a
  five-key-shaped `runbook:` and `{name,event,match}` `tests:` that
  `test_detection` runs.

## 6. Non-goals

- **No detection deployment.** vala writes a validated, tested rule to the
  detections directory and leaves shipping it to the user's pipeline. It ships no
  detections of its own; exemplars are for shape, not deployment.
- **No matching semantics.** How a condition or modifier evaluates is
  [SPEC-0005](SPEC-0005-detection-engine.md).
- **No multi-doc authoring.** Field tools edit one rule per file (R-0006-07).

## 7. Open questions

- Should the field tools auto-call `validate_detection`/`test_detection` and
  refuse to save a rule that fails, rather than relying on the agent to run them
  (R-0006-04/05)?
- Should `reference_detection` exemplars expand beyond AWS to Windows/EDR
  sources?

## 8. References

- [SPEC-0005](SPEC-0005-detection-engine.md) — the engine these tools call.
- [SPEC-0004](SPEC-0004-hunting-workflow.md) — recording and linking the produced detection.
- `internal/sigma/edit.go` — the comment-preserving editor.
