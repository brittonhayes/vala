<h1 align="center">vala</h1>

<p align="center">
  <a href="https://pkg.go.dev/github.com/brittonhayes/vala"><img src="https://pkg.go.dev/badge/github.com/brittonhayes/vala.svg" alt="Go Reference"></a>
  <a href="https://github.com/brittonhayes/vala/actions/workflows/ci.yml"><img src="https://github.com/brittonhayes/vala/actions/workflows/ci.yml/badge.svg" alt="Build Status"></a>
  <a href="./LICENSE"><img src="https://img.shields.io/badge/license-MIT-green.svg" alt="License"></a>
</p>

<p align="center">A threat hunter that lives in your terminal. One binary, one loop: scope a hypothesis, hunt it down, reach a verdict, ship the detection.</p>

```
  vala  security detection & response · claude-opus-4-8
  type a request · /help for commands · "exit" to quit
  ────────────────────────────────────────────────────
  › a CISA advisory says GuardDuty is being disabled — hunt the last 24h
  › confirmed. author and link a Sigma detection for DeleteDetector
```

Point it at your data lake, describe the work, and walk away. Every hunt, every
finding, every detection lands in a Notion-backed brain so the next hunt builds
on the last. It runs on Anthropic's Claude and needs **no external detection
toolchain** — Sigma rules are validated and unit-tested offline, inside the
binary.

## Install

```sh
go install github.com/brittonhayes/vala/cmd/vala@latest
export ANTHROPIC_API_KEY=sk-ant-...
```

<details>
<summary>Build from source</summary>

```sh
git clone https://github.com/brittonhayes/vala && cd vala
go build -o vala ./cmd/vala
```

</details>

## Quickstart

Fire up the session and start talking:

```sh
vala
```

That's the whole surface — one interactive session. Three slash commands steer
the conversation itself:

| Command | What it does |
| --- | --- |
| `/help` | List the commands. |
| `/clear` | Wipe the context and transcript, keep the banner. |
| `/compact [focus]` | Summarize the session into a tight recap and keep going; `focus` steers it. |

Need a one-shot for CI or a cron? Same toolbox, no TTY:

```sh
vala run "validate and test every rule in my detections directory, report failures"
```

> [!TIP]
> Common flags: `--model <id>`, `--permission ask|allow|deny`, `--yes`.
> vala also auto-compacts as a turn approaches the context window (80% by
> default) so long sessions never run out of room — tune it with
> `context_window` / `auto_compact_threshold`, or set either to `0` to turn it off.

## Give it a brain

By default vala runs in ephemeral, in-memory mode — fine for a quick look,
forgotten the moment you quit. Wire it to Notion once and your hunts, intel,
evidence, and detections persist as one connected graph:

```sh
vala init --parent <notion-page-id>
```

`init` provisions the brain's databases under that page and writes their
data-source IDs into `.vala.json`. It's idempotent — re-run it any time to
verify and reuse what's there rather than duplicating it.

> [!NOTE]
> `init` needs an authenticated [Notion CLI](https://github.com/makenotion/ntn)
> (`ntn login`). Until you run it, vala reminds you on startup that it's in
> memory-only mode (silence it with `--no-init-prompt`).

## The hunt loop

vala runs a single loop, shaped after the frameworks every hunt team knows —
Sqrrl's Hunting Loop, Splunk PEAK, TaHiTI. The loop and its rationale are
specified in [`docs/SPEC-0001`](docs/SPEC-0001-overview-and-hunt-loop.md); the
full specification set — the grounding truth for what vala offers — lives in
[`docs/`](docs/README.md).

**1 · Scope.** State the hypothesis with ABLE — the testable adversary
**B**ehavior and the data-source **L**ocation where it'd show up. `recall` reads
the brain back first, so settled ground isn't re-hunted and related intel gets
pulled forward. `queue_hunt` drops triggers (intel, a hunch, a fresh CVE, a past
incident) onto a prioritized backlog.

**2 · Hunt.** `open_hunt` kicks off a hypothesis-driven hunt over read-only data
sources, spoken via the [Model Context Protocol](https://modelcontextprotocol.io).
Point it at a [Scanner](https://scanner.dev) data lake and it discovers the
indexes and fields, queries them, and records each fact as an immutable Finding
and each indicator, TTP, or actor as a first-class artifact.

**3 · Conclude.** `store_hunt` writes the narrative page with a verdict —
**Confirmed**, **Refuted**, or **Inconclusive**. Every declarative finding must
cite a Finding ID, or the page bounces.

**4 · Automate.** A confirmed hunt earns a detection. vala studies, authors,
validates, and tests a Sigma rule for the proven behavior — then links it back
to the hunt. A refuted hunt earns none.

## Detections

A confirmed hunt's output is a [Sigma](https://sigmahq.io) rule — vendor-neutral
detection-as-code that compiles to most SIEM backends. vala writes it to your
`detections_dir` (default `detections`) and leaves deployment to your pipeline.
It ships none of its own.

Two optional, schema-valid fields make each rule stand on its own:

- **`runbook:`** — inline response guidance (`triage`, `investigate`, `contain`,
  `escalate`, `references`).
- **`tests:`** — `{name, event, match}` cases vala runs through its offline
  engine to prove the rule's logic.

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

The offline engine in [`internal/detect`](internal/detect) checks rules against
the official Sigma schema and supports the common modifiers (`contains`,
`startswith`, `endswith`, `all`, `re`, `cidr`, `lt|lte|gt|gte`), `*`/`?`
wildcards, dotted field lookups, and `1 of` / `all of` quantifiers. The
reference rules under
[`internal/reference/sigma/`](internal/reference/sigma) are complete examples.

## Configuration

Settings layer lowest-priority first: built-in defaults →
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

**Evidence sources.** vala's evidence comes from MCP servers under `mcp`. Each is
dialed at startup over streamable HTTP, its tools discovered, and the read-only
ones handed to the agent; its bearer token is read from `api_key_env` and never
persisted. The reference source is [Scanner](https://scanner.dev), whose inverted
indexes make exploratory queries fast and cheap. As a shortcut, `SCANNER_MCP_URL`
(plus `SCANNER_API_KEY`) registers Scanner with no config file. With no MCP
server, vala reasons only over local files.

**Notion IDs.** The `notion` values are data-source IDs — `vala init` fills them
in for you. To wire databases by hand, resolve an ID with
`ntn datasources resolve <id>` and match each property name to the keys vala
writes (`hunt_id`, `status`, `started_at`, relations like `hunts`/`detections`).
Leave them empty to stay local.

> [!WARNING]
> Every non-read-only tool call is gated by `--permission`: `ask` (default)
> prompts per call, `allow` auto-approves for unattended runs, `deny` blocks all
> writes. Reach for `allow` only when you trust the run.

## Development

```sh
go build ./...
go vet ./...
go test ./...
```

CI runs build, vet, `go test -race`, and a `gofmt` check on every push and pull
request. The architecture follows
[charmbracelet/crush](https://github.com/charmbracelet/crush) — one `Tool` type
plus one embedded `.md` description per tool, a permission gate, sessions. The
registry in [`internal/tools/toolbox.go`](internal/tools/toolbox.go) is the
single extension point.

## License

[MIT](LICENSE) © Britton Hayes
