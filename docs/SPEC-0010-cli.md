# SPEC-0010 · CLI

> One binary: an interactive session by default, a one-shot `run`, guided
> onboarding via `setup`, and `version` — plus a handful of REPL commands.

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
- **Setup wizard** — `vala setup`, the guided onboarding flow that connects a
  provider, provisions or repairs the brain, and wires up evidence; also
  auto-launched by bare `vala` when something required is not yet configured.

## 3. Requirements

### Commands

- **R-0010-01** `vala` (no subcommand) MUST start the interactive REPL: load
  config, build the LLM client, gate, MCP evidence, and toolbox, and run the TUI
  with the permission gate and session recording.
- **R-0010-02** `vala run <prompt...>` MUST run a single task non-interactively
  over the same toolbox, joining args into one prompt, recording a transcript,
  and never prompting at a TTY.
- **R-0010-03** `vala setup` MUST run the guided onboarding wizard: connect a
  provider, choose and provision the brain, and wire up evidence. For the brain,
  it MUST offer an on-disk ("local") `brain_file` or a Notion brain; choosing
  Notion MUST prompt for the parent page ID, provision the single "Vala Brain"
  database (see [SPEC-0002](SPEC-0002-brain-and-persistence.md)), and save the
  IDs to `./.vala.json`. If the Notion CLI is not authenticated, the wizard MUST
  suspend and run `ntn login` before provisioning. There is no separate `init`
  command.
- **R-0010-04** `vala version` MUST print the build version (set via `-ldflags`,
  falling back to VCS info or `dev`).
- **R-0010-12** `vala connect [provider]` MUST guide the operator through
  selecting a provider and entering a credential (a masked API key for remote
  providers, a base URL for local ones), persist the credential to the store
  (`~/.config/vala/auth.json`) and the chosen `provider`/`model` to `./.vala.json`
  (see [SPEC-0009](SPEC-0009-configuration.md)). A named provider preselects it.
- **R-0010-14** For a provider that supports a subscription login (OAuth, e.g.
  Anthropic Claude Pro/Max), `vala connect` MUST offer signing in with the
  subscription as an alternative to pasting an API key. Choosing it MUST open
  (or print) the provider's browser consent URL, accept the pasted one-time
  code, exchange it for tokens, and store an OAuth credential — never a raw key.

### Flags

- **R-0010-05** `--model <id>` MUST override the config `model`; `--permission
  <mode>` MUST override the config `permission`; `--mode <id>` MUST override the
  config `mode` (`hunt`/`detect`; see [SPEC-0014](SPEC-0014-modes-and-skills.md));
  an empty flag value MUST leave the config value in place. An unknown `--mode`
  MUST fail with an error listing the valid ids.
- **R-0010-06** `vala run` MUST default to denying non-read-only tools and MUST
  auto-approve all calls when `--yes` is given (equivalent to permission
  `allow`).
- **R-0010-07** When a Notion brain is already configured, `vala setup` MUST
  verify it and repair it in place rather than duplicate it: if one or more data
  sources are missing or unreachable it MUST re-create only the missing
  store(s) under the existing "Vala Brain" database, and if the parent database
  itself is gone it MUST re-provision a fresh single database (see
  [SPEC-0002](SPEC-0002-brain-and-persistence.md)). A configured-but-incomplete
  brain is treated as a broken state setup proactively offers to fix.
- **R-0010-08** `--no-init-prompt` MUST suppress the first-run notice;
  `--require-brain` MUST make a missing brain a hard error instead of a warning.

### First run

- **R-0010-09** When no brain is configured, vala MUST notify the operator it is
  in ephemeral memory-only mode. At an interactive TTY it MUST offer to run
  `vala setup`; non-interactively it MUST print the notice and continue (never
  block automation).
- **R-0010-10** A dismissed first-run prompt MUST be remembered (in
  `~/.config/vala/state.json`) so it is not shown again.

### REPL commands

- **R-0010-13** The interactive session MUST provide `/connect [provider]`: bare,
  it lists providers and their connection state; with a provider id it switches
  the active provider live (carrying the conversation), optionally storing an
  inline credential. When no provider is connected at startup, the REPL MUST
  still launch so `/connect` can wire one up.
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
| `vala setup` | guided onboarding (provider, brain, evidence); provisions or repairs the brain | + persistent |
| `vala connect [provider]` | connect/select an LLM provider | + persistent |
| `vala version` | print version | — |

| Persistent flag | Effect |
|---|---|
| `--model <id>` | override `model` |
| `--permission <ask\|allow\|deny>` | override `permission` |
| `--mode <hunt\|detect>` | override `mode` (SPEC-0014) |
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
| `/connect [provider]` | list providers, or switch the active provider live (optionally storing an inline credential) |
| `/mode [id]` | list modes, or switch the active mode live (SPEC-0014) |
| `/clear` | wipe context and transcript, keep the banner |
| `/compact [focus]` | summarize the session and continue; `focus` steers the summary |

## 5. Acceptance criteria

- **A-0010-01** (R-0010-01/02) `vala` starts the REPL; `vala run "x"` runs once
  and exits without a TTY prompt.
- **A-0010-02** (R-0010-05) `--model m` makes the agent use model `m` regardless
  of config; an empty `--permission` leaves config's value.
- **A-0010-03** (R-0010-06) `vala run` denies a `write` by default and permits it
  under `--yes`.
- **A-0010-04** (R-0010-07) `vala setup` against a valid Notion config verifies
  and reuses the existing "Vala Brain" database; against a config missing a data
  source it re-creates only the missing store and never duplicates the database.
- **A-0010-05** (R-0010-09) With no brain and a non-interactive invocation, vala
  prints the notice and proceeds; with `--require-brain` it exits non-zero.
- **A-0010-06** (R-0010-11) `/help`, `/clear`, `/compact` are recognized in the
  REPL and are not present in the agent's tool registry.

## 6. Non-goals

- **No additional subcommands.** The surface is `run`, `setup`, `connect`,
  `version`, and the default REPL.
- **No scripting DSL.** One-shot automation is `vala run` with a natural-language
  prompt.

## 7. Open questions

- Should `vala run` support reading the prompt from stdin / a file for long
  tasks?

## 8. References

- [SPEC-0009](SPEC-0009-configuration.md) — the config these flags override.
- [SPEC-0002](SPEC-0002-brain-and-persistence.md) — what `vala setup` provisions.
- [SPEC-0011](SPEC-0011-permissions-and-safety.md) — `--permission`/`--yes` semantics.
