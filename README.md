# vala

An agentic threat-hunting system in a single Go binary.

vala runs one loop: scope a hypothesis, hunt it, reach a verdict, and write the
detection a confirmed hunt warrants. Every step is recorded in Notion.

It runs on Anthropic's Claude and needs no external detection toolchain: Sigma
rules are validated and unit-tested offline, inside the binary — no `sigma-cli`,
no `yq`, no Python.

## Quickstart

```sh
go install github.com/brittonhayes/vala/cmd/vala@latest
export ANTHROPIC_API_KEY=sk-ant-...
```

Provision the Notion-backed brain once, so your hunts, intel, evidence, and
detections persist instead of living in an ephemeral in-memory store. This needs
an authenticated [Notion CLI](https://github.com/makenotion/ntn) (`ntn login`):

```sh
vala init --parent <notion-page-id>
```

`init` creates the brain's databases under that page and writes their data-source
IDs into `.vala.json`. It is idempotent — re-running verifies and reuses the
existing databases rather than duplicating them. Until you run it, vala warns on
startup that it is in ephemeral in-memory mode (suppress with `--no-init-prompt`).

vala has a single surface: an interactive session. Start it and describe the
work:

```sh
vala
```

```
› queue a hunt: a CISA advisory says GuardDuty is being disabled — behavior
  DeleteDetector, data source cloudtrail
› hunt whether anyone disabled GuardDuty in the last 24h, and store the hunt
› now that it's confirmed, author and link a Sigma detection for that behavior
```

Inside the session, a few slash commands manage the conversation itself rather
than the agent:

| Command | Effect |
| --- | --- |
| `/help` | List the available commands. |
| `/clear` | Clear the conversation context and transcript, keeping the banner. |
| `/compact [focus]` | Summarize the conversation into a structured recap and continue with far fewer tokens; optional `focus` text steers the summary. |

vala also compacts optimistically on its own: when a turn's prompt approaches the
context window (80% by default), it summarizes before continuing so long sessions
don't run out of room. Tune this with `context_window` / `auto_compact_threshold`
(or `VALA_CONTEXT_WINDOW` / `VALA_AUTO_COMPACT_THRESHOLD`); set either to `0` to
disable auto-compaction.

Run a one-shot task non-interactively (same toolbox, no TTY):

```sh
vala run "validate and test every rule in my detections directory, and report failures"
```

Common flags: `--model <id>`, `--permission ask|allow|deny`, `--yes`.

> **Build from source:** `git clone https://github.com/brittonhayes/vala && cd
> vala && go build -o vala ./cmd/vala`

## The hunt loop

vala runs one loop, following the shape established hunt frameworks share
(Sqrrl's Hunting Loop, Splunk PEAK, TaHiTI). See
[`docs/threat-hunting-system.md`](docs/threat-hunting-system.md) for the
rationale.

**1 · Scope.** State the hypothesis with ABLE — the testable adversary
**B**ehavior and the data-source **L**ocation where it would appear. `recall`
reads the brain back first — prior hunts, intel, and detections — so settled
ground isn't re-hunted and related intel is pulled forward; this is what makes
each hunt compound on the last. `queue_hunt` records a trigger (intel, a hunch, a
fresh CVE, a past incident) on a prioritized **backlog**.

**2 · Hunt.** `open_hunt` starts a hypothesis-driven hunt. vala explores
read-only data sources over the Model Context Protocol — point it at a
[Scanner](https://scanner.dev) data lake and it discovers the available indexes
and fields (`scanner_load_context`), then queries them (`scanner_execute_query`),
recording each fact as an immutable Finding pointer and each indicator, TTP, or
actor as a first-class artifact.

**3 · Conclude.** `store_hunt` writes the narrative page with a Confirmed,
Refuted, or Inconclusive verdict. Every declarative finding must cite a Finding
ID, or the page is rejected.

**4 · Automate.** A confirmed hunt produces a detection. vala authors a Sigma
rule for the proven behavior — study, author, validate, test, document — editing
one field at a time to preserve comments and key order, checking it against the
official Sigma JSON schema offline, and running its inline `tests:` through a
built-in evaluation engine before linking it back to the hunt. A refuted hunt
produces none. vala ships no detections of its own; point it at your directory
with `detections_dir` (default `detections`).

In Notion, backlog, intel, hunts, and detections form one connected graph.

## Detections

A confirmed hunt's output is a [Sigma](https://sigmahq.io) rule — vendor-neutral
detection-as-code that converts to most SIEM backends. vala writes it to your
`detections_dir` and leaves deployment to your pipeline.

It populates two optional, schema-valid fields so the rule stands on its own:

- **`runbook:`** — inline response guidance (`triage`, `investigate`, `contain`,
  `escalate`, `references`).
- **`tests:`** — `{name, event, match}` cases vala runs through its offline
  evaluation engine to check the rule's logic.

```yaml
detection:
  selection:
    eventName: ConsoleLogin
    userIdentity.type: Root
  condition: selection
tests:
  - name: root console login fires
    event: { eventName: ConsoleLogin, userIdentity.type: Root }
    match: true
  - name: iam user login is ignored
    event: { eventName: ConsoleLogin, userIdentity.type: IAMUser }
    match: false
```

The offline engine (`internal/detect`) validates rules against the official Sigma
schema and supports the common modifiers (`contains`, `startswith`, `endswith`,
`all`, `re`, `cidr`, `lt|lte|gt|gte`), `*`/`?` wildcards, dotted field lookups,
and the `1 of` / `all of` quantifiers. The embedded reference rules under
[`internal/reference/sigma/`](internal/reference/sigma) are complete examples.

## Configuration

Settings layer (lowest priority first): built-in defaults →
`~/.config/vala/config.json` → `./.vala.json` → environment variables
(`ANTHROPIC_API_KEY`, `VALA_MODEL`, `VALA_PERMISSION`, `VALA_CONTEXT_WINDOW`,
`VALA_AUTO_COMPACT_THRESHOLD`, `SCANNER_MCP_URL`, `SCANNER_API_KEY`).

```json
{
  "model": "claude-opus-4-8",
  "permission": "ask",
  "detections_dir": "detections",
  "context_window": 200000,
  "auto_compact_threshold": 0.8,
  "mcp": [
    { "name": "scanner", "url": "https://<your>.scanner.dev/mcp", "api_key_env": "SCANNER_API_KEY" }
  ],
  "notion": {
    "evidence": "", "hunts": "", "intel": "", "detections": "", "backlog": ""
  }
}
```

### Evidence sources (MCP)

vala's evidence comes from [Model Context Protocol](https://modelcontextprotocol.io)
servers listed under `mcp`. Each server is dialed at startup over streamable
HTTP, its tools are discovered, and the read-only ones are exposed to the agent
during investigation and hunts; its bearer token is read from the environment
variable named by `api_key_env`, never persisted. The reference source is
[Scanner](https://scanner.dev), whose inverted indexes make exploratory queries
fast and cheap: `scanner_load_context` discovers indexes and fields,
`scanner_execute_query` runs an ad-hoc query, and `scanner_get_query_results`
pages rows without flooding the model's context. As a shortcut, setting
`SCANNER_MCP_URL` (plus `SCANNER_API_KEY`) registers Scanner without a config
file. With no MCP server configured, vala has no remote evidence source and can
only reason over local files.

The `notion` values are **data-source IDs**; the easiest way to populate them is
`vala init`, which provisions the databases with the exact property names and
types vala expects and writes the IDs here for you. To wire up databases by hand
instead, resolve a database ID with `ntn datasources resolve <id>` and ensure
each data source's property names match the field keys vala writes (`hunt_id`,
`status`, `started_at`, relations like `hunts`/`detections`, …). Leave the IDs
empty to run in local mode.

Every non-read-only tool call is gated by `--permission`: `ask` (default) prompts
per call, `allow` auto-approves for unattended runs, `deny` blocks all writes.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

CI runs build, vet, `go test -race`, and a `gofmt` check on every push and pull
request.

The architecture follows [charmbracelet/crush](https://github.com/charmbracelet/crush)
(one `Tool` type + one embedded `.md` description per tool, a permission gate,
sessions). The tool registry (`internal/tools/toolbox.go`) is the single
extension point.

## License

[MIT](LICENSE) © Britton Hayes
