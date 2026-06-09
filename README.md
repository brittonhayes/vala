# vala

An agentic threat-hunting system in a single Go binary.

vala runs one loop: scope a hypothesis, hunt it, reach a verdict, and write the
detection a confirmed hunt warrants. Every step is recorded in Notion. Hand it an
alert instead and it investigates, proposes actions, and writes an auditable
case, executing nothing you haven't approved.

It runs on Anthropic's Claude and needs no external detection toolchain: Sigma
rules are validated and unit-tested offline, inside the binary — no `sigma-cli`,
no `yq`, no Python.

## Quickstart

```sh
go install github.com/brittonhayes/vala/cmd/vala@latest
export ANTHROPIC_API_KEY=sk-ant-...
```

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
› work the alert in tests/ops/sample_alert.json
```

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
read-only data sources, recording each fact as an immutable Finding pointer and
each indicator, TTP, or actor as a first-class artifact.

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

### Responding to alerts

Hand it an alert and `open_case` drives it through a phase-separated governance
loop:

```
plan ─► evidence ─► propose ─► approval ─► execute ─► report
```

Each phase exposes a smaller set of tools. Investigation is read-only and records
immutable Evidence pointers; the agent proposes Actions but can't execute them; a
human or policy approves each one; only approved actions run, each at most once;
the final case page is rejected unless every claim cites evidence. The result is
an auditable case record in Notion. Without configured Notion database IDs, vala
runs in local mode and prints artifacts to stdout.

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
(`ANTHROPIC_API_KEY`, `VALA_MODEL`, `VALA_PERMISSION`, `VALA_ENV`,
`SLACK_WEBHOOK_URL`).

```json
{
  "model": "claude-opus-4-8",
  "permission": "ask",
  "detections_dir": "detections",
  "env": "dev",
  "notion": {
    "alerts": "", "cases": "", "evidence": "", "actions": "", "runs": "",
    "hunts": "", "intel": "", "detections": "", "backlog": "",
    "case_page_parent": ""
  }
}
```

`env` selects the policy environment (`dev`/`prod`). The `notion` values are
**data-source IDs** (resolve a database ID with `ntn datasources resolve <id>`);
set them to enable real Notion reads and writes, or leave them empty to run in
local mode. vala reads each data source's schema and types properties to match,
so each data source's property names should match the field keys vala writes
(`hunt_id`, `status`, `started_at`, relations like `hunts`/`detections`, …).
Every non-read-only tool call is gated by `--permission`: `ask` (default) prompts
per call, `allow` auto-approves for unattended runs, `deny` blocks all writes.

## Development

```sh
go build ./...
go vet ./...
go test ./...
go run ./cmd/vala harness --fixtures tests
```

CI runs build, vet, `go test -race`, the adversarial harness (diffed against
`runner/baseline.json`), and a `gofmt` check on every push and pull request.

The architecture follows [charmbracelet/crush](https://github.com/charmbracelet/crush)
(one `Tool` type + one embedded `.md` description per tool, a permission gate,
sessions). The tool registry (`internal/tools/default.go`) is the single
extension point.

## License

[MIT](LICENSE) © Britton Hayes
