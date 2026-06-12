# SPEC-0013 · Maturity & Autonomy

> A single Hunting Maturity Model dial (0–4) tunes how much vala does before
> pausing for the operator — by setting a permission default and the prompt's
> gating framing. It is an autonomy dial, not a new mode system: the loop and the
> tools are identical at every level.

| Field | Value |
|---|---|
| **ID** | SPEC-0013 |
| **Status** | Stable |
| **Updated** | 2026-06-10 |
| **Source of truth** | `internal/config/config.go`, `internal/cmd/root.go`, `internal/agent/prompt.go` |
| **Depends on** | SPEC-0009, SPEC-0011, SPEC-0001 |

## 1. Purpose & scope

This spec defines the **maturity** setting: the `maturity` config key and
`VALA_MATURITY` environment variable, how a level maps to a default permission
mode, how an explicit permission overrides it, and the per-level autonomy framing
appended to the system prompt. It maps vala's levels onto the Sqrrl Hunting
Maturity Model (HMM).

It does **not** redefine the permission gate itself (that is
[SPEC-0011](SPEC-0011-permissions-and-safety.md)) nor the config layering (that
is [SPEC-0009](SPEC-0009-configuration.md)); it is the autonomy dial layered on
top of both.

## 2. Definitions

- **Maturity (HMM level)** — an integer `0–4` naming the Hunting Maturity Model
  level the harness runs at. Default `1`.
- **Permission mode** — `ask` | `allow` | `deny`, the gate over non-read-only
  tools (see [SPEC-0011](SPEC-0011-permissions-and-safety.md)).
- **Maturity framing** — the `# Operating maturity` section the prompt appends,
  describing how much the agent should do before pausing.

## 3. Requirements

- **R-0013-01** The config MUST expose a `maturity` integer key (HMM level 0–4),
  defaulting to `1`, overridable by the `VALA_MATURITY` environment variable
  (parsed as an integer; a malformed value leaves the prior value unchanged).
- **R-0013-02** When no explicit permission is set anywhere (config file,
  `VALA_PERMISSION`, or `--permission`), the default permission mode MUST be
  derived from the maturity level via `MaturityPermission(level)`:

  | HMM level | Sqrrl stage | Default permission |
  |---|---|---|
  | 0 | Initial | `deny` |
  | 1 | Minimal | `ask` |
  | 2 | Procedural | `ask` |
  | 3 | Innovative | `allow` |
  | 4 | Leading | `allow` |

- **R-0013-03** An explicit permission MUST always win over the maturity-derived
  default. The derivation MUST run at the end of config load and fill the
  permission *only when it is empty*; an explicit value (config, `VALA_PERMISSION`,
  or the `--permission` flag applied after load) leaves it non-empty and is
  therefore not overwritten.
- **R-0013-04** The system prompt MUST append a `# Operating maturity` section
  whose text matches the level band (via `maturityFraming(level)`):
  - **HMM0** — investigate and propose only: draft and queue hypotheses, lay out
    the hunt, but execute no writes; the operator approves each step.
  - **HMM1–2** — run the standard hunt procedures end to end, confirm each write,
    and pause for review at the decide/convert stage before authoring or changing
    a detection.
  - **HMM3–4** — operate autonomously across the full loop, pausing only for
    genuinely destructive or outward-facing actions, and sample its own work so
    the operator can spot-check.
- **R-0013-05** Maturity MUST be an autonomy dial only: it MUST NOT add or remove
  commands or tools, and MUST NOT itself select a behavioral mode. Within a given
  mode the loop, the tools, and the lint gates are identical at every level; only
  the default permission mode and the prompt framing change. Maturity is
  orthogonal to modes ([SPEC-0014](SPEC-0014-modes-and-skills.md)): a mode chooses
  *what* the agent does, maturity chooses *how much* it does before pausing. This
  preserves the intent of [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md)
  R-0001-04 — maturity adds no behavior of its own.

## 4. Behavior & interfaces

### Config & derivation

`config.Default()` sets `Maturity: 1` and `Permission: ""` (empty = derive).
`config.Load` reads `VALA_MATURITY` and, at the very end — after the config file,
the environment, and before the `--permission` flag is applied by the command
layer — fills the permission from maturity *only if still empty*:

```go
if cfg.Permission == "" {
    cfg.Permission = MaturityPermission(cfg.Maturity)
}
```

`MaturityPermission`: `level<=0 → "deny"`, `level>=3 → "allow"`, else `"ask"`.

The command layer (`internal/cmd/root.go`) applies `--permission` after `Load`
and before constructing the gate, so the flag (like any explicit value) wins:

```
flag/env/config Permission set ──► wins
otherwise empty ──► MaturityPermission(Maturity)
```

`permission.New(permission.Parse(cfg.Permission), cfg.Allowlist)` then builds the
gate; the maturity level changes nothing else about how the gate evaluates a
call.

### Prompt framing

`agent.SystemPrompt(workdir, toolNames, maturityLevel, operatorContext)` and
`agent.New(..., maturityLevel, ...)` take the level and append the
`maturityFraming(level)` section. The framing is the *soft* expression of the
autonomy the permission gate enforces *hard* — they describe the same posture,
one to the model and one to the harness.

### Mapping to the Hunting Maturity Model

vala's five levels are the Sqrrl HMM: HMM0 initial (relies on automated
alerting, little hunting), HMM1 minimal, HMM2 procedural, HMM3 innovative, HMM4
leading (highly automated). The dial lets one binary serve a team anywhere on
that curve — read-only investigation at HMM0, hands-on confirmation in the
middle, autonomous operation at HMM3–4 — without changing what vala *is*.

## 5. Acceptance criteria

- **A-0013-01** (R-0013-02) `MaturityPermission(0)=="deny"`,
  `MaturityPermission(1)==MaturityPermission(2)=="ask"`,
  `MaturityPermission(3)==MaturityPermission(4)=="allow"`.
- **A-0013-02** (R-0013-03) With `Permission` set explicitly, `Load` leaves it
  unchanged regardless of `Maturity`; with `Permission` empty, `Load` sets it to
  `MaturityPermission(Maturity)`.
- **A-0013-03** (R-0013-01) `VALA_MATURITY=3` sets `cfg.Maturity` to 3 and, with
  no explicit permission, yields permission `allow`.
- **A-0013-04** (R-0013-04) The system prompt contains a `# Operating maturity`
  section whose text differs across the HMM0 / HMM1–2 / HMM3–4 bands
  (`internal/agent/prompt_test.go` `TestSystemPromptMaturityFraming`).
- **A-0013-05** (R-0013-05) The tool set and loop framing in the prompt are
  identical across maturity levels; only the `# Operating maturity` section
  changes (`TestSystemPromptMaturityFraming`, `TestSystemPromptEnumeratesLoopAndTiers`).

## 6. Non-goals

- **No new commands or tools.** Maturity adds none; it is permission default +
  framing only (R-0013-05).
- **No per-tool maturity rules.** The gate is permission-mode + allowlist
  ([SPEC-0011](SPEC-0011-permissions-and-safety.md)); maturity only sets the
  default mode, it does not introduce per-tool maturity thresholds.
- **No auto-promotion.** vala does not advance the maturity level based on
  observed behavior; the operator sets it.

## 7. Open questions

- Should HMM1 and HMM2 ever differ (e.g. HMM2 allowing detection authoring
  without per-write confirmation), or remain a single `ask` band?
- Should the gate's destructive/outward-action carve-out at HMM3–4 be encoded in
  the permission layer rather than only described in the prompt framing?

## 8. References

- [SPEC-0009](SPEC-0009-configuration.md) — config schema and layering.
- [SPEC-0011](SPEC-0011-permissions-and-safety.md) — the permission gate.
- [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) R-0001-04, R-0001-14 — modes vs. autonomy.
- [SPEC-0014](SPEC-0014-modes-and-skills.md) — modes and skills (orthogonal to maturity).
- [The Cyber Hunting Maturity Model — Sqrrl](https://medium.com/@sqrrldata/the-cyber-hunting-maturity-model-6d506faa8ad5)
- `internal/config/config.go` — `Maturity`, `MaturityPermission`, `Load`.
- `internal/agent/prompt.go` — `maturityFraming`.
