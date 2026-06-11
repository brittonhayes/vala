# SPEC-0009 · Configuration

> Settings layer defaults → user config → project config → environment, with
> secrets resolved from the environment and never persisted.

| Field | Value |
|---|---|
| **ID** | SPEC-0009 |
| **Status** | Stable |
| **Updated** | 2026-06-11 |
| **Source of truth** | `internal/config/config.go`, `internal/config/save.go`, `internal/llm/new.go`, `internal/llm/registry.go`, `internal/auth/auth.go` |
| **Depends on** | SPEC-0002 |

## 1. Purpose & scope

This spec defines vala's configuration: every setting, its default, the
precedence between sources, and the environment variables that override them. It
fixes how secrets are handled (environment-only) and how `vala setup` persists
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

- **R-0009-04** A provider API key MUST come only from the environment (the
  provider's key variable, e.g. `ANTHROPIC_API_KEY` / `OPENAI_API_KEY`) or the
  credential store, and MUST NOT be persisted to any config file. The credential
  store is `~/.config/vala/auth.json` (mode `0600`), written by `vala connect`;
  the environment variable, when set, MUST take precedence over the store.
- **R-0009-11** A provider MAY be connected with a subscription (OAuth) login
  instead of an API key. The browser-based PKCE flow stores an
  `access`/`refresh`/`expiry` triple (credential `type` = `oauth`) in the same
  `0600` store; no raw key is ever entered or persisted. An OAuth credential
  MUST be refreshed automatically when within the refresh window of expiry, the
  rotated tokens written back to the store. When a provider has an OAuth
  credential, it takes precedence over key lookup. Anthropic (Claude Pro/Max) is
  the first provider to support this; OAuth-authenticated Messages API requests
  MUST send the access token as a Bearer credential (never `x-api-key`), attach
  the `anthropic-beta: oauth-2025-04-20` header, and lead the system prompt with
  Claude Code's identity block.
- **R-0009-05** No MCP server secret MUST be persisted to `.vala.json`. An HTTP
  server's bearer token is resolved at load time from its `api_key_env` variable;
  a stdio server's passthrough variables from the names in `env`; an `oauth: true`
  HTTP server holds no secret in config at all (its token is cached out of band,
  see [SPEC-0007](SPEC-0007-evidence-and-mcp.md) R-0007-04). An MCP server's
  `transport` defaults to `http` when unset; a `stdio` server reads `command`,
  `args`, and `env` instead of `url`/`api_key_env`.
- **R-0009-10** A non-local provider with no key (neither environment nor store)
  MUST surface as a recoverable "not connected" condition so the interactive
  session can launch and offer `/connect`, while unattended runs fail closed.

### Defaults

- **R-0009-06** Defaults MUST be: `provider=anthropic`, `model=claude-opus-4-8`, `max_tokens=8192`,
  `permission=""` (empty — derived from `maturity`, see R-0009-13), `maturity=1`,
  `detections_dir=detections`, `max_steps=50`, `context_window=200000`,
  `auto_compact_threshold=0.80`, empty `notion`, empty `mcp`, nil `allowlist`.
- **R-0009-07** An empty `notion` block MUST select the in-memory brain
  ([SPEC-0002](SPEC-0002-brain-and-persistence.md) R-0002-09).
- **R-0009-13** `maturity` is the Hunting Maturity Model level (`0–4`, default
  `1`), overridable by `VALA_MATURITY` (malformed value ignored). When
  `permission` is left unset (empty after all layers), it MUST be **derived** from
  `maturity` via `config.MaturityPermission` at the end of `Load`:
  `0 → deny`, `1–2 → ask`, `3–4 → allow`. An explicit `permission` — from config,
  `VALA_PERMISSION`, or the `--permission` flag — always wins, since it leaves
  `permission` non-empty and the derivation only fills an empty value. See
  [SPEC-0013](SPEC-0013-maturity-and-autonomy.md) for the full maturity model.

### Convenience & persistence

- **R-0009-08** If `SCANNER_MCP_URL` is set and no `scanner` server is already
  configured, vala MUST register a `scanner` server using that URL and
  `SCANNER_API_KEY`.
- **R-0009-09** `vala setup` MUST persist the provisioned Notion IDs — the
  parent `database` ID, each store's data-source ID, and `page_parent` — to
  `./.vala.json` under `notion` via `config.SaveNotion`, preserving every other
  key in the file byte-for-byte.

## 4. Behavior & interfaces

### Config schema

| Field (JSON) | Type | Default | Env override | Consumed by |
|---|---|---|---|---|
| `provider` | string | `anthropic` | `VALA_PROVIDER` | SPEC-0008 |
| `providers` | map[string]ProviderConfig | empty | — | SPEC-0008 |
| `model` | string | `claude-opus-4-8` | `VALA_MODEL` | SPEC-0008 |
| `max_tokens` | int64 | `8192` | — | SPEC-0008 |
| `permission` | string | `""` (derived from `maturity`) | `VALA_PERMISSION` | SPEC-0011 |
| `maturity` | int | `1` | `VALA_MATURITY` | SPEC-0013 |
| `allowlist` | []string | nil | — | SPEC-0011 |
| `detections_dir` | string | `detections` | — | SPEC-0006 |
| `max_steps` | int | `50` | — | SPEC-0008 |
| `context_window` | int64 | `200000` | `VALA_CONTEXT_WINDOW` | SPEC-0008 |
| `auto_compact_threshold` | float64 | `0.80` | `VALA_AUTO_COMPACT_THRESHOLD` | SPEC-0008 |
| `notion` | `brain.DBIDs` | empty | — (set by `vala setup`) | SPEC-0002 |
| `mcp` | []MCPServer | nil | `SCANNER_MCP_URL` (append) | SPEC-0007 |
| `APIKey` | string | from env | `ANTHROPIC_API_KEY` | SPEC-0008 |

### Environment variables

| Variable | Effect |
|---|---|
| `<provider>_API_KEY` | the active provider's key (e.g. `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`); never persisted, takes precedence over the credential store |
| `VALA_PROVIDER` | override `provider` |
| `VALA_MODEL` | override `model` |
| `VALA_PERMISSION` | override `permission` (`ask`/`allow`/`deny`); wins over the maturity-derived default |
| `VALA_MATURITY` | override `maturity` (int 0–4; malformed ignored); sets the default `permission` when none is set explicitly |
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

### Providers & model resolution

vala supports any provider speaking one of two wire protocols — `anthropic`
(Anthropic Messages) or `openai` (OpenAI Chat Completions). Built-in providers
(`anthropic`, `openai`, `google`, `openrouter`, `groq`, `deepseek`, `xai`,
`ollama`, `lmstudio`) need no config. The `providers` map overrides a built-in or
defines a new OpenAI-compatible provider:

```json
"providers": {
  "mygateway": {
    "base_url": "https://gateway.internal/v1",
    "protocol": "openai",
    "api_key_env": "GATEWAY_KEY",
    "model": "my-model",
    "local": false
  }
}
```

- **R-0009-11** The active provider is `provider`; the active model is `model`,
  interpreted within that provider. When `provider` is empty, a `provider/model`
  prefix in `model` selects the provider only if the prefix is a known built-in
  (so OpenRouter slugs like `anthropic/claude-opus-4-8` stay intact); otherwise
  the provider defaults to `anthropic`. An empty `model` uses the provider's
  default model.
- **R-0009-12** A `providers` entry overrides the matching built-in field by
  field; an entry with no matching built-in defines a provider outright, with the
  `openai` protocol assumed and `base_url` required.

### Credential store (`~/.config/vala/auth.json`)

```json
{
  "providers": {
    "openai": { "type": "api", "key": "...", "model": "gpt-5" },
    "anthropic": { "type": "oauth", "access": "...", "refresh": "...", "expiry": 1700000000000, "model": "claude-opus-4-8" }
  }
}
```

Written by `vala connect` (and `/connect`) at mode `0600`. A `type` of `api`
holds a key; `type` `oauth` holds a subscription login's `access`/`refresh`
tokens and `expiry` (Unix ms), refreshed in place as needed. `base_url` may be
set for a local or custom provider; `key` is omitted for local providers.
Secrets never appear in `config.json` / `.vala.json`.

### Notion IDs (`brain.DBIDs`)

```json
"notion": {
  "database": "",
  "evidence": "", "hunts": "", "intel": "",
  "detections": "", "backlog": "", "memory": "", "coverage": "",
  "page_parent": ""
}
```

`database` is the parent "Vala Brain" database ID; each store key holds that
store's data-source ID; `page_parent` is the brain's home page (where narrative
hunt pages are written). Empty → in-memory brain. `vala setup` fills these in
(`config.SaveNotion` overlays only the `notion` key, pretty-printed, creating
the file if absent).

### Example `./.vala.json`

```json
{
  "provider": "anthropic",
  "model": "claude-opus-4-8",
  "permission": "ask",
  "detections_dir": "detections",
  "context_window": 200000,
  "auto_compact_threshold": 0.8,
  "mcp": [
    { "name": "scanner", "url": "https://acme.scanner.dev/mcp", "api_key_env": "SCANNER_API_KEY" }
  ],
  "notion": { "database": "...", "evidence": "...", "hunts": "...", "intel": "...", "detections": "...", "backlog": "...", "memory": "...", "coverage": "...", "page_parent": "..." }
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
- **A-0009-08** (R-0009-13) With no `permission` set, `Load` derives it from
  `maturity` (`0→deny`, `1–2→ask`, `3–4→allow`); with `permission` set explicitly
  (config, `VALA_PERMISSION`, or `--permission`), `Load` leaves it unchanged
  regardless of `maturity`; `VALA_MATURITY=3` and no explicit permission yields
  `allow`.

## 6. Non-goals

- **No runtime reload.** Config is read at startup; live reload is out of scope.
- **No secret storage.** vala stores no secrets; tokens live in the environment.

## 7. Open questions

- Should `max_tokens`, `max_steps`, and `detections_dir` also be env-overridable
  for parity with the other knobs?

## 8. References

- [SPEC-0002](SPEC-0002-brain-and-persistence.md) — `DBIDs` and brain selection.
- [SPEC-0010](SPEC-0010-cli.md) — flags that override `model`/`permission`.
- [SPEC-0011](SPEC-0011-permissions-and-safety.md) — the permission gate `permission` feeds.
- [SPEC-0013](SPEC-0013-maturity-and-autonomy.md) — `maturity` and the permission derivation.
