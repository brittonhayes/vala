# SPEC-0014 · Modes & Skills

> A **mode** is a selectable specialization the harness runs in — a system
> prompt, an exposed tool subset, and a set of bundled **skills** — turning vala
> into "Claude Code, but for defensive security": one agent and toolbox, focused
> per workflow. **hunt** is the default and reproduces the classic full loop;
> **detect** focuses on detection authoring. A **skill** is a Claude-Code-style
> capability pack (a `SKILL.md`) a mode bundles and the agent loads on demand.

| Field | Value |
|---|---|
| **ID** | SPEC-0014 |
| **Status** | Stable |
| **Updated** | 2026-06-12 |
| **Source of truth** | `internal/mode/`, `internal/skills/`, `internal/agent/agent.go`, `internal/agent/prompt.go`, `internal/tools/skill.go` |
| **Depends on** | SPEC-0001, SPEC-0003, SPEC-0008, SPEC-0009, SPEC-0010 |

## 1. Purpose & scope

This spec defines **modes** and **skills**: the `mode` config key, the `--mode`
flag, the `VALA_MODE` environment variable, the `/mode` REPL command, how a mode
shapes the system prompt and the exposed tool set, and how skills are discovered,
listed for progressive disclosure, and loaded in full. It supersedes the original
"no modes" framing of [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) R-0001-02/-04:
modes are an intentional evolution, and **hunt** preserves the prior behavior
byte-for-byte as the default.

It does NOT change the hunt loop (SPEC-0001), the tool harness (SPEC-0003), the
brain (SPEC-0002), or the maturity/autonomy dial (SPEC-0013) — modes are
orthogonal to maturity: a mode chooses *what* the agent does, maturity *how much*.

## 2. Concepts

- **Mode** — `internal/mode.Mode`: an `ID`, `Title`, `Description`, an `Intro`
  (prompt headline), a `PromptBody` (workflow-specific prompt section), a
  `ToolPolicy` (which registered tools are exposed; nil = all), optional autonomy
  defaults, and a list of bundled skill ids.
- **hunt** — the default mode. `ToolPolicy` is nil (every tool exposed), it
  bundles no skills, and its prompt is the eight-stage loop of SPEC-0001.
- **detect** — Detection Engineering. Drops the hunt-lifecycle/intel-graph tools
  (`open_hunt`, `validate_data`, `store_hunt`, `update_coverage`, `queue_hunt`,
  `record_finding`, `record_intel`, `link_artifacts`); centers the
  detection-authoring toolkit; bundles the `sigma-authoring` skill.
- **Skill** — `internal/skills.Skill`: a `SKILL.md` with YAML frontmatter
  (`name`, `description`) and a markdown body. The name must match `^[a-z0-9-]+$`
  and equal its directory name.

## 3. Requirements

### Modes

- **R-0014-01** vala MUST run in exactly one mode at a time, resolved from the
  `mode` config key. Precedence MUST be: built-in default (`hunt`) → user config →
  project config → `VALA_MODE` → `--mode`. An unknown id MUST fail with a clear
  error listing the valid ids.
- **R-0014-02** The **hunt** mode MUST reproduce the pre-modes system prompt and
  exposed tool set byte-for-byte for identical inputs (guarded by a golden test in
  `internal/agent`). Introducing modes MUST NOT change default behavior.
- **R-0014-03** A mode MUST shape the agent by (a) supplying the system prompt's
  headline and workflow body and (b) filtering which registered tools are exposed
  to the model. It MUST NOT build a separate registry — there is one registry per
  session (SPEC-0003), so the permission gate and MCP evidence are unaffected.
- **R-0014-04** MCP evidence tools MUST stay exposed in **every** mode regardless
  of the mode's tool policy. The `skill` tool MUST be exposed exactly when the
  active mode bundles at least one skill. The agent MUST enforce both structurally
  (not rely on each mode's policy to remember them).
- **R-0014-05** A tool hidden by the active mode MUST NOT execute even if the
  model names it directly; the agent MUST return an error result instead.
- **R-0014-06** The `/mode` REPL command MUST list the modes (marking the active
  one) and, given an id, switch the active mode in place — recomputing the system
  prompt and exposed tool set without losing the conversation (mirroring
  `/connect`). Switching MUST be refused mid-turn.
- **R-0014-07** A mode MAY declare default permission/maturity; an explicitly
  configured value (config, env, or flag) MUST always win. (The shipped modes
  declare none, so no autonomy change ships with this increment.)

### Skills

- **R-0014-08** Skills MUST be discovered from three roots, later overriding
  earlier by name: builtin (embedded in the binary), `<user config>/vala/skills/`,
  and `<workdir>/.vala/skills/`. Each lives at `<root>/<name>/SKILL.md`.
- **R-0014-09** Discovery MUST be best-effort: a missing root is skipped, and a
  malformed `SKILL.md` (no frontmatter, bad YAML, or `name` ≠ directory) MUST be
  skipped with a warning, never fail the session.
- **R-0014-10** The system prompt MUST list the active mode's bundled skills by
  name and description only (progressive disclosure); full bodies MUST NOT be
  inlined. The `skill` tool MUST return a body in full on demand and MUST list the
  available skills when asked.
- **R-0014-11** A skill body is operator-curated guidance and is followed as
  instruction, but anything it quotes from logs, files, or external data MUST
  still be treated as untrusted DATA (the standing rule of SPEC-0001).

## 4. Behavior & interfaces

```
config/env/flag ─► mode.Get(id) ─► agent.Session{Mode, Skills, EvidenceNames}
                                         │
   skills.Load(workdir) ────────────────┤  agent.New / SetMode → applyMode:
   (builtin + user + project)           │    • activeTool = modeFilter(mode)
                                         │    • system = SystemPrompt(mode, …, skills)
                                         ▼
   loop exposes registry.ToolDefsFiltered(activeTool); runToolUse refuses hidden tools
```

- Config/flag/env: `internal/config/config.go` (`Mode`), `internal/cmd/root.go`
  (`--mode`, `VALA_MODE` via Load, `build()` resolves the mode and loads skills).
- Agent: `internal/agent/agent.go` (`Session`, `New`, `SetMode`, `applyMode`,
  `modeFilter`, `exposedToolNames`), `internal/agent/prompt.go` (`SystemPrompt`,
  `skillsSection`).
- REPL: `internal/ui/commands.go` (`/mode`).
- Tool: `internal/tools/skill.go` (the `skill` tool), registered in
  `internal/tools/toolbox.go`.

## 5. Acceptance

- `hunt` golden prompt test passes (R-0014-02); `detect` prompt omits the hunt
  loop and lists bundled skills.
- Tool-filter tests: `detect` hides hunt-lifecycle tools, exposes
  detection/`skill`, and keeps a stub MCP evidence tool; `SetMode` swaps live;
  `runToolUse` refuses a hidden tool without running it.
- Skills discovery test: finds the builtin, a project skill overrides a same-named
  builtin, malformed skills are skipped with warnings, builtin name == directory.
- `--mode bogus` and `VALA_MODE=bogus` produce a friendly unknown-mode error.

## 6. Open questions

- Should modes be discoverable from the filesystem (a `MODE.md`), or stay built-in?
- Should a mode be able to add tools (not just filter), e.g. a mode-specific tool?
- The roadmap modes — **dart** (DART/IR) and **report-review** (threat-report
  triage) — slot in by appending to `internal/mode.builtins()`; what tool policies
  and skills should they carry?

## 7. References

- [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) — the hunt loop (the hunt mode).
- [SPEC-0003](SPEC-0003-tool-harness.md) — the single tool registry modes filter.
- [SPEC-0008](SPEC-0008-agent-and-session.md) — the agent loop modes configure.
- [SPEC-0009](SPEC-0009-configuration.md) — config schema and layering.
- [SPEC-0013](SPEC-0013-maturity-and-autonomy.md) — maturity (orthogonal to modes).
- `internal/mode/`, `internal/skills/`, `internal/agent/`, `internal/tools/skill.go`.
