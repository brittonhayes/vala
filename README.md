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
on the last. It is **provider-agnostic** — run it on Anthropic's Claude, OpenAI,
Google Gemini, OpenRouter, or a local model — and needs **no external detection
toolchain**: Sigma rules are validated and unit-tested offline, inside the binary.

## Install

```sh
go install github.com/brittonhayes/vala/cmd/vala@latest
vala            # first run opens guided setup automatically
```

The first time you run `vala` and it isn't fully wired up, it opens a guided
setup that walks you through the three things it needs — a **model provider**, a
**brain** (where findings persist), and the **evidence sources** it hunts in
(Scanner, Wiz, or any MCP server). There's nothing separate to find; vala knows
when you're not set up and helps you finish. Run `vala setup` anytime to add a
source or change a choice.

For Anthropic you can **log in with your Claude Pro/Max subscription** — vala
opens your browser, you paste back the one-time code, and no raw API key is ever
entered or stored. Prefer a key? Paste one instead, or skip setup entirely: a key
already in your environment (e.g. `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`) is
picked up automatically. To set up just the provider, `vala connect` jumps
straight to the provider picker.

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
| `/connect [provider]` | Choose or switch the model provider mid-session; lists providers when bare. |
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

## Choose your provider

vala talks to whatever model backend you point it at. One binary, any provider:

```sh
vala connect              # guided picker (masked key entry)
vala connect openai       # jump straight to a provider
vala connect ollama       # point at a local server — no key needed
```

| Provider | Models | Auth |
| --- | --- | --- |
| `anthropic` | Claude (Opus, Sonnet, Haiku) | `ANTHROPIC_API_KEY` |
| `openai` | GPT-5, GPT-4.1, o4-mini | `OPENAI_API_KEY` |
| `google` | Gemini 2.5 Pro/Flash | `GEMINI_API_KEY` |
| `openrouter` | any model, one key | `OPENROUTER_API_KEY` |
| `groq` · `deepseek` · `xai` | Llama, DeepSeek, Grok | provider key |
| `ollama` · `lmstudio` | local models | none (local server) |

Under the hood there are just two wire protocols — Anthropic Messages and OpenAI
Chat Completions — so every OpenAI-compatible endpoint (including local servers
and private gateways) works by pointing at a base URL. Switch providers live in
a session with `/connect`; the conversation carries over. Define a custom
OpenAI-compatible provider under `providers` in `.vala.json`:

```json
{
  "provider": "mygateway",
  "model": "my-model",
  "providers": {
    "mygateway": { "base_url": "https://gateway.internal/v1", "api_key_env": "GATEWAY_KEY" }
  }
}
```

Credentials live in `~/.config/vala/auth.json` (mode `0600`), never in the
project config; environment variables always take precedence.

## Give it a brain

By default vala runs in ephemeral, in-memory mode — fine for a quick look,
forgotten the moment you quit. Give it a persistent brain and your hunts, intel,
evidence, and detections compound across sessions. Just run vala — on first
launch it detects what isn't set up and opens a guided setup where you pick a
brain (re-run it any time with `vala setup`):

```sh
vala            # first launch opens setup automatically
vala setup      # re-run to add, change, or repair a piece
```

- **On-disk brain** — durable, no account. Writes the brain to a single JSON file
  (`.vala/brain.json` by default) and records the path in `.vala.json`; narrative
  hunt pages land as readable Markdown beside it. The zero-dependency path —
  nothing to log into, and the whole brain is a portable, version-controllable
  artifact.
- **Notion brain** — shared with your team. Provisions a single **Vala Brain**
  database with one data source per store (hunts, evidence, intel, detections,
  backlog, memory, coverage) under a Notion page you choose, and writes the
  data-source IDs into `.vala.json`. If a store ever goes missing, re-running
  setup repairs it in place rather than leaving the brain half-broken.

> [!NOTE]
> The Notion path needs the [Notion CLI](https://github.com/makenotion/ntn);
> setup runs `ntn login` for you when you aren't authenticated yet. The on-disk
> path needs nothing. Until a brain is configured, vala reminds you on startup
> that it's in memory-only mode (silence it with `--no-init-prompt`).

## Give it context

vala opens every session with standing context about your environment, from two
places:

**`VALA.md`** is what you write by hand — a plain Markdown file vala reads into
every session: crown-jewel systems, where each log source lives, what "normal"
looks like, detection naming conventions, prior incidents. Setup drops a
commented starter in your project; fill in what matters.

```
your-repo/VALA.md        ← project context, version-controlled with the team
~/.config/vala/VALA.md   ← personal context, merged in first
```

**Shared memory** is what vala learns as it hunts. When the agent discovers a
durable fact — "auth logs live in Okta", "svc-deploy rotates keys nightly" — it
calls `remember`, which writes that fact to the brain stamped with who learned
it. Point a whole team at the same Notion brain and memory becomes **multiplayer**:
one hunter's discovery primes everyone's next session. With a local brain it's
still yours across sessions; ephemeral mode forgets it on exit.

Unlike query results and file contents — which vala treats as untrusted data —
both VALA.md and team memory are operator-authored, so vala trusts them as
guidance.

## The hunt loop

vala runs a single eight-stage loop, shaped after the frameworks every hunt team
knows — Sqrrl's Hunting Loop, Splunk PEAK, TaHiTI. The loop and its rationale are
specified in [`docs/SPEC-0001`](docs/SPEC-0001-overview-and-hunt-loop.md); the
full specification set — the grounding truth for what vala offers — lives in
[`docs/`](docs/README.md).

**1 · Scope & prioritize.** Choose what to hunt — weighting detection coverage
gaps first, then active threat intel, then what matters in this environment.
`recall` (including the `coverage` scope) reads the brain back so settled ground
isn't re-hunted; `queue_hunt` parks triggers on a prioritized backlog.

**2 · Form hypothesis.** State it with ABLE — the testable adversary
**B**ehavior and the data-source **L**ocation where it'd show up — and pick a
hunt type: hypothesis-driven, baseline, or model-assisted. `open_hunt` opens it.

**3 · Plan & validate data.** `validate_data` confirms the telemetry exists
before you query. A failed check is recorded as a visibility gap — never a
silent skip — and is an actionable outcome in its own right.

**4–5 · Execute & deep-dive.** Investigate over read-only data sources, spoken
via the [Model Context Protocol](https://modelcontextprotocol.io). Point it at a
[Scanner](https://scanner.dev) data lake and it discovers the indexes, queries
them, baselines normal, and records each fact as an immutable Finding (and each
indicator, TTP, or actor as a first-class artifact).

**6 · Document & decide.** `store_hunt` writes the narrative page with a verdict
— **Confirmed**, **Refuted**, or **Inconclusive** — and a detection-output tier
decision. Every declarative finding must cite a Finding ID, or the page bounces.

**7 · Convert to detection.** vala picks the highest-fidelity output the finding
supports: a Sigma rule (tiers 1–2), a recurring hunt (tier 3), a playbook (tier
4), or a justified no-build (tier 5). A low-value rule is worse than none.

**8 · Feed back.** `update_coverage` records the technique's coverage state so
the next hunt aims where coverage is weakest, and follow-on hypotheses are
queued. The autonomy at each stage scales with the Hunting Maturity Model level
([`docs/SPEC-0013`](docs/SPEC-0013-maturity-and-autonomy.md)).

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
(`VALA_PROVIDER`, `VALA_MODEL`, `VALA_PERMISSION`, `VALA_CONTEXT_WINDOW`,
`VALA_AUTO_COMPACT_THRESHOLD`, `SCANNER_MCP_URL`, `SCANNER_API_KEY`, and each
provider's own key env such as `ANTHROPIC_API_KEY` / `OPENAI_API_KEY`).

```json
{
  "provider": "anthropic",
  "model": "claude-opus-4-8",
  "permission": "ask",
  "detections_dir": "detections",
  "context_window": 200000,
  "auto_compact_threshold": 0.8,
  "mcp": [
    { "name": "scanner", "url": "https://<your>.scanner.dev/mcp", "api_key_env": "SCANNER_API_KEY" },
    { "name": "wiz", "url": "https://mcp.app.wiz.io/", "oauth": true }
  ],
  "brain_file": "",
  "notion": {
    "evidence": "", "hunts": "", "intel": "", "detections": "", "backlog": "", "memory": ""
  }
}
```

**Evidence sources.** vala's evidence comes from MCP servers under `mcp` — this is
what it actually hunts in. The fastest way to connect one is the guided setup,
which runs automatically the first time vala detects it isn't fully wired up, and
on demand any time:

```sh
vala setup     # connect a provider, a brain, and evidence sources
```

The wizard ships curated presets for [Scanner](https://scanner.dev) (a hosted
data lake over HTTPS with an API key) and [Wiz](https://www.wiz.io) (the Security
Graph, which you sign into in your browser), plus a custom-HTTP option for any MCP
server. It validates each connection live — you see `✓ N tools` before you leave
— and writes the entry to `.vala.json`. You can also hand-edit the `mcp` array:
each server is connected at startup over its `transport` (`http`, the default, or
`stdio`), its tools discovered, and the read-only ones handed to the agent.
Secrets are never persisted to `.vala.json`:

- a bearer-token server reads its key from `api_key_env`;
- an `"oauth": true` server (e.g. Wiz) signs in through the browser on first use
  and caches its token under `~/.config/vala/mcp-auth.json` (mode `0600`),
  refreshing it silently on later launches;
- a `stdio` server reads the variables named in `env` and passes them to the
  subprocess.

As a shortcut, `SCANNER_MCP_URL` (plus `SCANNER_API_KEY`) registers Scanner with
no config file. The session banner shows which sources connected; with none, vala
reasons only over local files.

**Notion IDs.** The `notion` values are the parent `database` plus one
data-source ID per store — `vala setup` fills them in for you. To wire them by
hand, resolve an ID with
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
