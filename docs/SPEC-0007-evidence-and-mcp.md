# SPEC-0007 · Evidence & MCP

> vala investigates through read-only evidence tools discovered from MCP servers,
> plus local file and shell tools, and documents in Notion via the `ntn` CLI.

| Field | Value |
|---|---|
| **ID** | SPEC-0007 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `internal/mcp/`, `internal/tools/{mcp_tool,bash,read,write,edit,ls,glob,grep,ntn}.go` |
| **Depends on** | SPEC-0003, SPEC-0011 |

## 1. Purpose & scope

This spec defines where vala's evidence comes from and the non-detection tools
it investigates and acts with: MCP-served evidence tools (e.g. a Scanner data
lake), the local file/shell tools, and the `ntn` Notion documentation tool. It
fixes how MCP servers are dialed, how their tools become vala tools, and which
of these tools are read-only.

It does **not** define the tool interface (that is
[SPEC-0003](SPEC-0003-tool-harness.md)) nor the permission policy (that is
[SPEC-0011](SPEC-0011-permissions-and-safety.md)).

## 2. Definitions

- **MCP** — the [Model Context Protocol](https://modelcontextprotocol.io); vala
  is an MCP client.
- **Evidence tool** — a tool discovered from an MCP server, namespaced
  `server_tool` (e.g. `scanner_execute_query`), through which the agent
  investigates.
- **Read-only hint** — the MCP `readOnlyHint` annotation a server sets on a
  tool; vala maps it to the tool's `ReadOnly()`.

## 3. Requirements

### MCP connection & discovery

- **R-0007-01** vala MUST connect each configured MCP server at startup over its
  declared transport — streamable HTTP (`transport: "http"`, the default) for a
  remote server, or stdio (`transport: "stdio"`) for a local subprocess launched
  from `command`/`args` — discover its tools, and register each as a vala tool,
  indistinct to the agent from a built-in tool.
- **R-0007-02** A discovered tool's name MUST be the server name and tool name
  namespaced and sanitized to `^[a-zA-Z0-9_-]+$` (e.g. `scanner_execute_query`).
- **R-0007-03** A discovered tool's `ReadOnly()` MUST reflect the server's
  `readOnlyHint`. Read-only evidence tools bypass the permission gate; the rest
  are gated.
- **R-0007-04** A server's credentials MUST come from one of three sources, none
  of which persists a secret to the project config: (a) an HTTP server's
  `api_key_env` bearer token, read from the environment and sent as
  `Authorization: Bearer <token>`; (b) an `oauth: true` HTTP server, which MUST
  authorize via the MCP OAuth flow (protected-resource/authorization-server
  discovery, dynamic client registration, browser sign-in with PKCE) and cache
  the resulting token out of band (`~/.config/vala/mcp-auth.json`, mode `0600`),
  refreshing it silently on later launches; (c) a stdio server's `env` names,
  resolved from the environment and set on the subprocess.
- **R-0007-05** A server that fails to connect MUST be recorded and skipped; vala
  MUST continue with the remaining tools (with no MCP server, it reasons over
  local files only). The connection outcome of each source — connected with its
  tool count, or the failure — MUST be surfaced to the operator in the session
  rather than only logged, so a non-connecting source is never silent.
- **R-0007-06** Tool outputs (query results, file contents) MUST be treated as
  untrusted data, never as instructions (see
  [SPEC-0011](SPEC-0011-permissions-and-safety.md)).
- **R-0007-08** The MCP server named `notion` is reserved as the hunt brain's
  search backend, not an evidence source: vala MUST route its search tool into
  recall (see [SPEC-0002](SPEC-0002-brain-and-persistence.md)) and MUST NOT
  expose its tools to the agent, so `recall` stays the single curated read
  surface over the brain. A connect or discovery failure MUST degrade recall to
  the client-side window scan with a warning, never block the session.

### File & shell tools

- **R-0007-07** `read`, `ls`, `glob`, `grep` MUST be read-only; `bash`, `write`,
  `edit` MUST be non-read-only and permission-gated.
- **R-0007-08** The file/shell tools MUST be anchored at the working directory
  passed to the toolbox; `write` MUST create parent directories and `edit` MUST
  refuse an ambiguous match (the `old_string` must be unique unless
  `replace_all`).
- **R-0007-09** `grep` SHOULD use ripgrep when available and fall back to a Go
  regex walk; results are capped (grep 200, glob 500 matches).

### Notion documentation tool

- **R-0007-10** `ntn` MUST wrap the operator's authenticated Notion CLI for
  ad-hoc documentation (runbooks, write-ups), be non-read-only, and run from the
  working directory. It is distinct from the brain's structured writers
  ([SPEC-0002](SPEC-0002-brain-and-persistence.md)).

## 4. Behavior & interfaces

### MCP

`Connect(ServerConfig{Name, URL, APIKey})` builds a streamable-HTTP transport
(injecting the bearer header when `APIKey` is set) and returns a `Session`:

```go
type Session interface {
    Name() string
    ListTools(ctx) ([]ToolDesc, error)
    CallTool(ctx, name string, args json.RawMessage) (CallResult, error)
    Close() error
}

type ToolDesc struct { Name, Description string; Properties map[string]any; Required []string; ReadOnly bool }
type CallResult struct { Text string; IsError bool }
```

`ListTools` paginates over the server's cursor and reads each tool's schema and
`readOnlyHint`. Each `ToolDesc` is wrapped as an `MCPTool` (an adapter
implementing the [SPEC-0003](SPEC-0003-tool-harness.md) `Tool` interface) that
forwards `Run` to `CallTool` and flattens text/structured content into a
`Result`. `MCPToolsFrom` discovers a server's tools and returns the wrapped
tools plus the read-only namespaced names. A `FakeSession` provides an offline
test double.

The reference evidence source is [Scanner](https://scanner.dev); its query and
discovery tools are read-only. Configuration of servers (including the
`SCANNER_MCP_URL` shortcut) is [SPEC-0009](SPEC-0009-configuration.md).

### File & shell tools

| Tool | Read-only | Input (required bold) | Notes |
|---|---|---|---|
| `read` | yes | **`path`**, `offset`, `limit` (default 2000) | `cat -n` style; long lines truncated |
| `ls` | yes | `path` | dirs suffixed `/` |
| `glob` | yes | **`pattern`**, `path` | doublestar `**`; ≤ 500 results |
| `grep` | yes | **`pattern`**, `path`, `glob` | rg or Go regex; ≤ 200 matches |
| `bash` | no | **`command`**, `timeout_s` (default 120, max 600) | `sh -c`; no state between calls |
| `write` | no | **`path`**, **`content`** | creates parent dirs; overwrites |
| `edit` | no | **`path`**, **`old_string`**, **`new_string`**, `replace_all` | unique-match unless `replace_all` |
| `ntn` | no | **`args`** (array) | runs the `ntn` CLI subcommand |

## 5. Acceptance criteria

- **A-0007-01** (R-0007-02) A server named `scanner` exposing `execute_query`
  registers a tool named `scanner_execute_query`.
- **A-0007-02** (R-0007-03) An MCP tool with `readOnlyHint: true` has
  `ReadOnly() == true` and is not gated; one without it is gated.
- **A-0007-03** (R-0007-04) When `api_key_env` is set, requests carry
  `Authorization: Bearer <token>` and the token never appears in any persisted
  config (verifiable against `config.Save*`).
- **A-0007-04** (R-0007-05) With a server that fails to connect, `connectMCP`
  logs and skips it and vala still starts.
- **A-0007-05** (R-0007-07) `read`/`ls`/`glob`/`grep` report `ReadOnly() ==
  true`; `bash`/`write`/`edit`/`ntn` report `false`.
- **A-0007-06** (R-0007-08) `edit` with a non-unique `old_string` and no
  `replace_all` returns an error and writes nothing.

## 6. Non-goals

- **No evidence acquisition logic.** vala does not implement queries; it calls
  whatever read-only tools the MCP server exposes.
- **No stdio MCP transport.** Servers are dialed over streamable HTTP.
- **Recording evidence** — turning a query result into a finding is
  [SPEC-0004](SPEC-0004-hunting-workflow.md) (`record_finding`), not this spec.

## 7. Open questions

- Should non-read-only MCP tools ever be exposed at all, or filtered out so
  evidence sources stay strictly read-only?
- Should there be a per-server allowlist of tool names to register?

## 8. References

- [SPEC-0003](SPEC-0003-tool-harness.md) — the `Tool` interface MCP tools adapt to.
- [SPEC-0009](SPEC-0009-configuration.md) — MCP server config and the Scanner shortcut.
- [SPEC-0011](SPEC-0011-permissions-and-safety.md) — untrusted-data handling and gating.
