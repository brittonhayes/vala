# vala specifications

This directory is the **specification set** for vala — the grounding truth for
what the tool offers and how each part is required to behave. The specs are
written to serve two readers at once:

1. **A human** who wants to know what vala does, precisely, without reading Go.
2. **A planner ⇄ implementer loop** doing spec-driven development: the planner
   edits a spec, the implementer makes the code match it. Every spec is written
   so that "does the code match the spec?" is an answerable question.

Specs describe **behavior and contracts**, not implementation. They name the
source of truth in the tree so an implementer can find the code, but a spec is
not a code listing — it is the set of promises that code must keep.

## Index

| Spec | Title | Scope |
|---|---|---|
| [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) | Overview & the Hunt Loop | What vala is; the Scope → Hunt → Conclude → Automate loop |
| [SPEC-0002](SPEC-0002-brain-and-persistence.md) | Brain & Persistence | The five-table graph, in-memory vs Notion, provisioning |
| [SPEC-0003](SPEC-0003-tool-harness.md) | Tool Harness | The `Tool` interface, registry, permission gating, the toolbox |
| [SPEC-0004](SPEC-0004-hunting-workflow.md) | Hunting Workflow | `recall`, `queue_hunt`, `open_hunt`, `record_finding`, `record_intel`, `link_artifacts`, `store_hunt` |
| [SPEC-0005](SPEC-0005-detection-engine.md) | Detection Engine | Offline Sigma validation, condition grammar, field matching, tests |
| [SPEC-0006](SPEC-0006-detection-authoring.md) | Detection Authoring | Reference exemplars, field-editing tools, runbook & tests |
| [SPEC-0007](SPEC-0007-evidence-and-mcp.md) | Evidence & MCP | Connecting evidence sources over MCP; file/shell tools |
| [SPEC-0008](SPEC-0008-agent-and-session.md) | Agent & Session | The tool-use loop, auto-compaction, transcripts, the LLM client |
| [SPEC-0009](SPEC-0009-configuration.md) | Configuration | Config schema, layering, environment variables |
| [SPEC-0010](SPEC-0010-cli.md) | CLI | `vala`, `vala run`, `vala init`, `vala version`, flags, first-run |
| [SPEC-0011](SPEC-0011-permissions-and-safety.md) | Permissions & Safety | The permission gate, untrusted data, secret handling |

## How to read a spec

Every spec follows the same skeleton (see [the template](#spec-template)):

- **Header table** — ID, status, the source-of-truth paths, and dependencies.
- **Purpose & Scope** — what the spec covers and, explicitly, what it does not.
- **Definitions** — terms used normatively in the spec.
- **Requirements** — numbered, normative statements (`R-XXXX-NN`). These use
  [RFC 2119](https://www.rfc-editor.org/rfc/rfc2119) keywords: **MUST** /
  **SHALL** (required), **SHOULD** (recommended), **MAY** (optional). A
  requirement is the contract; if the code disagrees with a MUST, the code is
  wrong (or the spec is — see [changing a spec](#changing-a-spec)).
- **Behavior & Interfaces** — the detail behind the requirements: data shapes,
  state machines, field names. Non-normative explanation lives here.
- **Acceptance criteria** — numbered, testable checks (`A-XXXX-NN`) an
  implementer can run to prove a requirement is met. Where a test already
  exists, the criterion points at it.
- **Non-goals / Open questions / References** — boundaries, undecided points,
  and external sources.

## Conventions

- **IDs are stable.** `SPEC-0004`, `R-0004-03`, and `A-0004-03` keep their
  numbers for life. Retire a requirement by marking it `withdrawn`, never by
  renumbering. New requirements take the next free number.
- **Status** is one of:
  - `Stable` — describes shipped behavior; the code is expected to match today.
  - `Draft` — proposed or in-progress; the code may not match yet. This is the
    planner's working surface.
  - `Withdrawn` — no longer in force, kept for history.
- **Source of truth** in the header points at the package(s) that implement the
  spec. It is a navigation aid, not a contract — the requirements are.
- **One capability per spec.** If a change spans specs, edit each and note the
  cross-reference; do not fold unrelated promises together.

## Spec-driven development with these specs

The loop these specs are built for:

1. **Plan.** The planner picks the spec(s) a change touches, edits the
   requirements and acceptance criteria to describe the desired behavior, and
   flips affected requirements (or the whole spec) to `Draft`.
2. **Implement.** The implementer treats the `Draft` requirements as the work
   order: make the code satisfy every MUST, then make each acceptance criterion
   pass. The acceptance criteria are the definition of done.
3. **Settle.** When the criteria pass, the spec returns to `Stable`. The diff to
   the spec and the diff to the code land together, so the spec never drifts
   from the tree.

Because requirements are small and individually testable, a change can be
scoped to a handful of `R-` IDs, and review can check the code against exactly
those lines.

## Changing a spec

A spec is wrong when it contradicts intended behavior — not when it contradicts
the current code. If the code is right and the spec is stale, fix the spec. If
the spec is right and the code is stale, that is a bug; file it or fix it.
Either way the two land in sync. When intent itself changes, edit the spec
**first** (that is the plan), then the code.

## Spec template

New specs copy this skeleton verbatim and fill it in. Keep the section order and
the ID scheme.

```markdown
# SPEC-XXXX · Title

> One-sentence summary of what this spec governs.

| Field | Value |
|---|---|
| **ID** | SPEC-XXXX |
| **Status** | Draft |
| **Updated** | YYYY-MM-DD |
| **Source of truth** | `internal/...` |
| **Depends on** | SPEC-YYYY |

## 1. Purpose & scope
What this spec covers. What it explicitly does not (point at the spec that does).

## 2. Definitions
Terms used normatively below.

## 3. Requirements
- **R-XXXX-01** The component MUST ...
- **R-XXXX-02** The component SHOULD ...

## 4. Behavior & interfaces
The detail behind the requirements: data shapes, state machines, field names,
worked examples. Non-normative.

## 5. Acceptance criteria
- **A-XXXX-01** (R-XXXX-01) Given ..., when ..., then ... — `go test ./...`
- **A-XXXX-02** (R-XXXX-02) ...

## 6. Non-goals
Out of scope, with a pointer to where it lives if anywhere.

## 7. Open questions
Undecided points, each owned or dated.

## 8. References
Specs, code, and external sources.
```
