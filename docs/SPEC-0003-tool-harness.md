# SPEC-0003 · Tool Harness

> Every capability vala has is a `Tool` in one registry, gated at call time by a
> permission check. The toolbox is the single extension point.

| Field | Value |
|---|---|
| **ID** | SPEC-0003 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `internal/tool/tool.go`, `internal/tools/toolbox.go` |
| **Depends on** | SPEC-0001 |

## 1. Purpose & scope

This spec defines the tool abstraction that the whole product is built on: the
`Tool` interface every capability implements, the `Registry` that holds them,
how tools are exposed to the model, and the toolbox that assembles vala's full
set. It is the contract a new tool must satisfy.

It does **not** specify any individual tool's behavior — those live in
[SPEC-0004](SPEC-0004-hunting-workflow.md) (hunting),
[SPEC-0006](SPEC-0006-detection-authoring.md) (authoring), and
[SPEC-0007](SPEC-0007-evidence-and-mcp.md) (evidence/file/shell). The permission
gate itself is [SPEC-0011](SPEC-0011-permissions-and-safety.md).

## 2. Definitions

- **Tool** — a primitive the agent can call: a name, a description, an input
  schema, a read-only flag, and a `Run` method.
- **Registry** — the named collection of tools available to an agent.
- **Toolbox** — the function that builds vala's one registry from the file/shell
  tools, detection tools, hunting tools, the `ntn` tool, and discovered MCP
  evidence tools.
- **Read-only tool** — a tool that only observes state; it bypasses the
  permission gate.
- **Result** — a tool's output: `{Content string, IsError bool}`. A tool error
  is reported to the model as a result, not by failing the turn.

## 3. Requirements

### The interface

- **R-0003-01** A tool MUST implement: `Name() string`, `Description() string`,
  `Schema() Schema`, `ReadOnly() bool`, and `Run(ctx, input json.RawMessage)
  (Result, error)`.
- **R-0003-02** A tool's `Name()` MUST match `^[a-zA-Z0-9_-]+$` and MUST be
  unique within a registry (a later registration with the same name replaces the
  earlier one).
- **R-0003-03** `Schema()` MUST return a JSON-Schema-shaped object
  (`{Properties, Required}`) describing the tool's input, suitable for the
  Anthropic tool API.
- **R-0003-04** `ReadOnly()` MUST return `true` if and only if the tool only
  observes state and never mutates the filesystem, the brain, an external
  service, or the host. This flag is the sole criterion the permission gate uses.
- **R-0003-05** A tool SHOULD surface operational errors as a `Result` with
  `IsError: true` (so the model can react) rather than returning a Go error,
  which is reserved for harness-level failures.
- **R-0003-06** A tool's `Description()` SHOULD be authored as an embedded `.md`
  file alongside the tool, keeping guidance reviewable and separate from code.

### The registry

- **R-0003-07** The registry MUST provide `Register(...Tool)`, `Get(name)`,
  `All()` (sorted by name), and conversion to Anthropic tool params.
- **R-0003-08** The registry MUST support a **filtered** conversion that drops
  tools by predicate, so the harness can present only permitted tools when the
  permission mode forbids the rest.

### The toolbox

- **R-0003-09** `Toolbox(dir, rc, evidence…)` MUST be the single place tools are
  registered — the one extension point for new capabilities.
- **R-0003-10** Registering a tool in the toolbox MUST NOT bypass the permission
  gate; exposure and authorization are independent. Every non-read-only tool is
  gated at call time regardless of how it was registered.
- **R-0003-11** The toolbox MUST register discovered MCP evidence tools
  (see [SPEC-0007](SPEC-0007-evidence-and-mcp.md)) alongside the built-in tools,
  indistinguishable to the agent.

## 4. Behavior & interfaces

### Interface

```go
type Schema struct {
    Properties map[string]any // JSON Schema "properties"
    Required   []string
}

type Result struct {
    Content string
    IsError bool
}

type Tool interface {
    Name() string
    Description() string
    Schema() Schema
    ReadOnly() bool
    Run(ctx context.Context, input json.RawMessage) (Result, error)
}
```

### Registry

```go
NewRegistry() *Registry
(*Registry) Register(tools ...Tool)
(*Registry) Get(name string) (Tool, bool)
(*Registry) All() []Tool                                   // sorted by name
(*Registry) ToAnthropic() []anthropic.ToolUnionParam
(*Registry) ToAnthropicFiltered(pred func(Tool) bool) []anthropic.ToolUnionParam
```

`ToAnthropicFiltered` is how the harness narrows the advertised tool set (e.g.
under `deny`, omit non-read-only tools).

### The toolbox contents

`Toolbox(dir string, rc *RunContext, evidence ...tool.Tool)` registers, in one
registry:

| Group | Tools | Read-only? | Spec |
|---|---|---|---|
| Evidence (MCP) | discovered, e.g. `scanner_execute_query` | per server hint | [SPEC-0007](SPEC-0007-evidence-and-mcp.md) |
| Shell + file | `bash`, `read`, `write`, `edit`, `ls`, `glob`, `grep` | read/ls/glob/grep yes; bash/write/edit no | [SPEC-0007](SPEC-0007-evidence-and-mcp.md) |
| Detection authoring | `reference_detection`, `validate_detection`, `test_detection`, `set_detection_meta`, `set_detection_logsource`, `edit_detection_logic`, `manage_detection_list`, `set_detection_runbook`, `manage_detection_tests` | the three readers yes; the editors no | [SPEC-0006](SPEC-0006-detection-authoring.md) |
| Hunting / brain | `recall`, `queue_hunt`, `open_hunt`, `record_finding`, `record_intel`, `link_artifacts`, `store_hunt` | `recall` yes; rest no | [SPEC-0004](SPEC-0004-hunting-workflow.md) |
| Notion | `ntn` | no | [SPEC-0007](SPEC-0007-evidence-and-mcp.md) |

Two arguments parameterize the box:
- `dir` anchors the file/shell and detection-authoring tools to a working root.
- `rc` is the session `RunContext` the hunting tools write through; `open_hunt`
  sets its active hunt at runtime (see [SPEC-0004](SPEC-0004-hunting-workflow.md)).

## 5. Acceptance criteria

- **A-0003-01** (R-0003-02) `Register` replaces a same-named tool; `All()`
  returns tools sorted by name with no duplicate names.
- **A-0003-02** (R-0003-04) For every registered tool, `ReadOnly()` is `true`
  exactly for tools that perform no mutation (auditable against the table above).
- **A-0003-03** (R-0003-08) `ToAnthropicFiltered(t => t.ReadOnly())` yields only
  read-only tools.
- **A-0003-04** (R-0003-09, R-0003-11) `Toolbox(...)` returns one registry
  containing the built-in groups above plus every tool in `evidence...`.
- **A-0003-05** (R-0003-10) A non-read-only tool returned by `Toolbox` is still
  refused under permission mode `deny` (cross-check with
  [SPEC-0011](SPEC-0011-permissions-and-safety.md)).

## 6. Non-goals

- **Tool *behavior*** — each tool's semantics belong to its own spec.
- **Permission *policy*** — how the gate decides belongs to
  [SPEC-0011](SPEC-0011-permissions-and-safety.md); this spec only fixes that
  `ReadOnly()` is the gate's input.
- **Streaming / partial results** — tools return a single `Result`.

## 7. Open questions

- Should the schema carry richer per-field validation (enums, bounds) that the
  harness enforces before `Run`, rather than each tool re-checking input?

## 8. References

- [SPEC-0011](SPEC-0011-permissions-and-safety.md) — the gate that consumes `ReadOnly()`.
- [SPEC-0008](SPEC-0008-agent-and-session.md) — the loop that calls `Run` and converts the registry for the API.
- `internal/tool/tool.go` — the interface and registry.
