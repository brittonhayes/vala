# SPEC-0010 · CLI

> One binary: an interactive session by default, a one-shot `run`, brain
> provisioning via `init`, and `version` — plus a handful of REPL commands.

| Field | Value |
|---|---|
| **ID** | SPEC-0010 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `cmd/vala/main.go`, `internal/cmd/`, `internal/ui/commands.go` |
| **Depends on** | SPEC-0008, SPEC-0009, SPEC-0011 |

## 1. Purpose & scope

This spec defines vala's command-line surface: the subcommands, their flags,
first-run behavior, and the in-session REPL commands. It fixes what each command
does and how flags override config.

It does **not** define the agent loop ([SPEC-0008](SPEC-0008-agent-and-session.md)),
provisioning internals ([SPEC-0002](SPEC-0002-brain-and-persistence.md)), or the
permission gate ([SPEC-0011](SPEC-0011-permissions-and-safety.md)).

## 2. Definitions

- **Interactive mode** — the default REPL session (`vala` with no subcommand).
- **One-shot** — `vala run "<prompt>"`, a single non-interactive task.
- **First-run notice** — the startup prompt shown when no Notion brain is
  configured.

## 3. Requirements

### Commands

- **R-0010-01** `vala` (no subcommand) MUST start the interactive REPL: load
  config, build the LLM client, gate, MCP evidence, and toolbox, and run the TUI
  with the permission gate and session recording.
- **R-0010-02** `vala run <prompt...>` MUST run a single task non-interactively
  over the same toolbox, joining args into one prompt, recording a transcript,
  and never prompting at a TTY.
- **R-0010-03** `vala init` MUST provision the Notion brain (see
  [SPEC-0002](SPEC-0002-brain-and-persistence.md)) under a parent page and save
  the IDs to `./.vala.json`; it MUST be idempotent and require an authenticated
  Notion CLI.
- **R-0010-04** `vala version` MUST print the build version (set via `-ldflags`,
  falling back to VCS info or `dev`).

### Flags

- **R-0010-05** `--model <id>` MUST override the config `model`; `--permission
  <mode>` MUST override the config `permission`; an empty flag value MUST leave
  the config value in place.
- **R-0010-06** `vala run` MUST default to denying non-read-only tools and MUST
  auto-approve all calls when `--yes` is given (equivalent to permission
  `allow`).
- **R-0010-07** `vala init` MUST accept `--parent <page-id>` (prompted if
  omitted) and `--force` (re-provision even if a brain is already configured).
- **R-0010-08** `--no-init-prompt` MUST suppress the first-run notice;
  `--require-brain` MUST make a missing brain a hard error instead of a warning.

### First run

- **R-0010-09** When no brain is configured, vala MUST notify the operator it is
  in ephemeral memory-only mode. At an interactive TTY it MUST offer to run
  `vala init`; non-interactively it MUST print the notice and continue (never
  block automation).
- **R-0010-10** A dismissed first-run prompt MUST be remembered (in
  `~/.config/vala/state.json`) so it is not shown again.

### REPL commands

- **R-0010-11** The interactive session MUST provide `/help` (list commands),
  `/clear` (wipe context and transcript, keep the banner), and `/compact
  [focus]` (summarize and continue, steered by optional focus — see
  [SPEC-0008](SPEC-0008-agent-and-session.md) R-0008-11). These steer the
  conversation only; they are not agent tools.

## 4. Behavior & interfaces

### Commands & flags

| Command | Purpose | Flags |
|---|---|---|
| `vala` | interactive REPL | persistent flags below |
| `vala run <prompt...>` | one-shot task | `--yes`, + persistent |
| `vala init` | provision Notion brain | `--parent`, `--force`, + persistent |
| `vala version` | print version | — |

| Persistent flag | Effect |
|---|---|
| `--model <id>` | override `model` |
| `--permission <ask\|allow\|deny>` | override `permission` |
| `--no-init-prompt` | suppress first-run notice |
| `--require-brain` | fail if no Notion brain configured |

### Build pipeline (interactive & run)

`resolveConfig` loads config for the cwd and applies `--model`/`--permission`.
`build` constructs the LLM client (error if no API key), the permission gate,
connects MCP servers (`connectMCP` dials each, discovers tools, logs+skips
failures), and assembles the toolbox. `brainStore` returns an `NTN` store when a
brain is configured, else `Mem`.

### REPL commands

| Command | Effect |
|---|---|
| `/help` | list the commands |
| `/clear` | wipe context and transcript, keep the banner |
| `/compact [focus]` | summarize the session and continue; `focus` steers the summary |

## 5. Acceptance criteria

- **A-0010-01** (R-0010-01/02) `vala` starts the REPL; `vala run "x"` runs once
  and exits without a TTY prompt.
- **A-0010-02** (R-0010-05) `--model m` makes the agent use model `m` regardless
  of config; an empty `--permission` leaves config's value.
- **A-0010-03** (R-0010-06) `vala run` denies a `write` by default and permits it
  under `--yes`.
- **A-0010-04** (R-0010-07) `vala init` re-run without `--force` against a valid
  config does not duplicate databases.
- **A-0010-05** (R-0010-09) With no brain and a non-interactive invocation, vala
  prints the notice and proceeds; with `--require-brain` it exits non-zero.
- **A-0010-06** (R-0010-11) `/help`, `/clear`, `/compact` are recognized in the
  REPL and are not present in the agent's tool registry.

## 6. Non-goals

- **No additional subcommands.** The surface is `run`, `init`, `version`, and the
  default REPL.
- **No scripting DSL.** One-shot automation is `vala run` with a natural-language
  prompt.

## 7. Open questions

- Should `vala run` support reading the prompt from stdin / a file for long
  tasks?

## 8. References

- [SPEC-0009](SPEC-0009-configuration.md) — the config these flags override.
- [SPEC-0002](SPEC-0002-brain-and-persistence.md) — what `vala init` provisions.
- [SPEC-0011](SPEC-0011-permissions-and-safety.md) — `--permission`/`--yes` semantics.
