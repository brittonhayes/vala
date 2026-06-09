# SPEC-0009 · Configuration

> Settings layer defaults → user config → project config → environment, with
> secrets resolved from the environment and never persisted.

| Field | Value |
|---|---|
| **ID** | SPEC-0009 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `internal/config/config.go`, `internal/config/save.go` |
| **Depends on** | SPEC-0002 |

## 1. Purpose & scope

This spec defines vala's configuration: every setting, its default, the
precedence between sources, and the environment variables that override them. It
fixes how secrets are handled (environment-only) and how `vala init` persists
the Notion IDs.

It does **not** define what the settings do at runtime — those belong to the
specs that consume them (model/compaction to
[SPEC-0008](SPEC-0008-agent-and-session.md), Notion IDs to
[SPEC-0002](SPEC-0002-brain-and-persistence.md), MCP to
[SPEC-0007](SPEC-0007-evidence-and-mcp.md), permission to
[SPEC-0011](SPEC-0011-permissions-and-safety.md)).

## 2. Definitions

- **User config** — `~/.config/vala/config.json` (via `os.UserConfigDir`).
- **Project config** — `./.vala.json` in the working directory.
- **Knob** — a single config field.

## 3. Requirements

### Layering

- **R-0009-01** Config MUST resolve in strict order, each layer overriding the
  previous: (1) built-in defaults, (2) user config, (3) project config, (4)
  environment variables.
- **R-0009-02** A missing config file MUST NOT be an error; malformed JSON in a
  present project config MUST be an error.
- **R-0009-03** Only the knobs listed in §4 as env-overridable MUST be
  overridable by environment variables; a malformed numeric env value MUST be
  ignored, leaving the prior value intact.

### Secrets

- **R-0009-04** The Anthropic API key MUST come only from `ANTHROPIC_API_KEY`
  and MUST NOT be persisted to any config file.
- **R-0009-05** Each MCP server's bearer token MUST be resolved at load time from
  the server's `api_key_env` variable and MUST NOT be persisted.

### Defaults

- **R-0009-06** Defaults MUST be: `model=claude-opus-4-8`, `max_tokens=8192`,
  `permission=ask`, `detections_dir=detections`, `max_steps=50`,
  `context_window=200000`, `auto_compact_threshold=0.80`, empty `notion`, empty
  `mcp`, nil `allowlist`.
- **R-0009-07** An empty `notion` block MUST select the in-memory brain
  ([SPEC-0002](SPEC-0002-brain-and-persistence.md) R-0002-09).

### Convenience & persistence

- **R-0009-08** If `SCANNER_MCP_URL` is set and no `scanner` server is already
  configured, vala MUST register a `scanner` server using that URL and
  `SCANNER_API_KEY`.
- **R-0009-09** `vala init` MUST persist the provisioned Notion IDs to
  `./.vala.json` under `notion`, preserving every other key in the file
  byte-for-byte.

## 4. Behavior & interfaces

### Config schema

| Field (JSON) | Type | Default | Env override | Consumed by |
|---|---|---|---|---|
| `model` | string | `claude-opus-4-8` | `VALA_MODEL` | SPEC-0008 |
| `max_tokens` | int64 | `8192` | — | SPEC-0008 |
| `permission` | string | `ask` | `VALA_PERMISSION` | SPEC-0011 |
| `allowlist` | []string | nil | — | SPEC-0011 |
| `detections_dir` | string | `detections` | — | SPEC-0006 |
| `max_steps` | int | `50` | — | SPEC-0008 |
| `context_window` | int64 | `200000` | `VALA_CONTEXT_WINDOW` | SPEC-0008 |
| `auto_compact_threshold` | float64 | `0.80` | `VALA_AUTO_COMPACT_THRESHOLD` | SPEC-0008 |
| `notion` | `brain.DBIDs` | empty | — (set by `vala init`) | SPEC-0002 |
| `mcp` | []MCPServer | nil | `SCANNER_MCP_URL` (append) | SPEC-0007 |
| `APIKey` | string | from env | `ANTHROPIC_API_KEY` | SPEC-0008 |

### Environment variables

| Variable | Effect |
|---|---|
| `ANTHROPIC_API_KEY` | the LLM API key (required; never persisted) |
| `VALA_MODEL` | override `model` |
| `VALA_PERMISSION` | override `permission` (`ask`/`allow`/`deny`) |
| `VALA_CONTEXT_WINDOW` | override `context_window` (int; malformed ignored) |
| `VALA_AUTO_COMPACT_THRESHOLD` | override `auto_compact_threshold` (float; malformed ignored) |
| `SCANNER_MCP_URL` | register a `scanner` MCP server if none configured |
| `SCANNER_API_KEY` | bearer token for the `scanner` server |
| `<api_key_env>` | bearer token for any MCP server naming it |

### MCP server config

```json
{ "name": "scanner", "url": "https://<host>/mcp", "api_key_env": "SCANNER_API_KEY" }
```

`api_key` is never written; it is resolved from `api_key_env` at load.

### Notion IDs (`brain.DBIDs`)

```json
"notion": {
  "evidence": "", "hunts": "", "intel": "",
  "detections": "", "backlog": "", "page_parent": ""
}
```

Empty → in-memory brain. `vala init` fills these in (`config.SaveNotion`
overlays only the `notion` key, pretty-printed, creating the file if absent).

### Example `./.vala.json`

```json
{
  "model": "claude-opus-4-8",
  "permission": "ask",
  "detections_dir": "detections",
  "context_window": 200000,
  "auto_compact_threshold": 0.8,
  "mcp": [
    { "name": "scanner", "url": "https://acme.scanner.dev/mcp", "api_key_env": "SCANNER_API_KEY" }
  ],
  "notion": { "evidence": "...", "hunts": "...", "intel": "...", "detections": "...", "backlog": "...", "page_parent": "..." }
}
```

## 5. Acceptance criteria

- **A-0009-01** (R-0009-01) A knob set in the project config overrides the user
  config, and the matching env var overrides both.
- **A-0009-02** (R-0009-02) `Load` on a directory with no `.vala.json` returns
  defaults and no error; a `.vala.json` with invalid JSON returns an error.
- **A-0009-03** (R-0009-03) `VALA_CONTEXT_WINDOW=abc` leaves `context_window` at
  its prior value.
- **A-0009-04** (R-0009-04/05) After `Load`, `APIKey` and each
  `MCPServer.APIKey` come from env; `config.Save*` output contains neither.
- **A-0009-05** (R-0009-06) `config.Default()` matches the defaults table.
- **A-0009-06** (R-0009-08) With `SCANNER_MCP_URL` set and no scanner server,
  `Load` appends one named `scanner` with `api_key_env=SCANNER_API_KEY`.
- **A-0009-07** (R-0009-09) `SaveNotion` overlays `notion` and leaves an existing
  unrelated key unchanged.

## 6. Non-goals

- **No runtime reload.** Config is read at startup; live reload is out of scope.
- **No secret storage.** vala stores no secrets; tokens live in the environment.

## 7. Open questions

- Should `max_tokens`, `max_steps`, and `detections_dir` also be env-overridable
  for parity with the other knobs?

## 8. References

- [SPEC-0002](SPEC-0002-brain-and-persistence.md) — `DBIDs` and brain selection.
- [SPEC-0010](SPEC-0010-cli.md) — flags that override `model`/`permission`.
