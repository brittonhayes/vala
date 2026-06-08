# vala

An agentic threat-hunting system — as a single Go binary.

A SIEM is something you search by hand. vala works the other way: it runs one
loop — **scope a hypothesis, hunt it, conclude, automate** — records every step
in Notion, and leaves a detection behind. Authoring a detection is not a separate
job; it is the *deliverable of a confirmed hunt*. Hand it an alert and it also
investigates, proposes actions, and writes an auditable case — without taking a
destructive action you didn't approve.

It runs on Anthropic's Claude and needs no external detection toolchain. Sigma
rules are validated and unit-tested natively and offline, inside the binary — no
`sigma-cli`, no `yq`, no Python.

## Quickstart

```sh
go install github.com/brittonhayes/vala/cmd/vala@latest
export ANTHROPIC_API_KEY=sk-ant-...
```

There is one surface: an interactive session with a toolbox. Start it and ask it
to do the work:

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

vala's spine is one loop, mapped to the way mature hunt teams work (Sqrrl's
Hunting Loop, Splunk PEAK, TaHiTI). See
[`docs/threat-hunting-system.md`](docs/threat-hunting-system.md) for the research
and rationale.

**1 · Scope.** Phrase the hypothesis with ABLE — the testable adversary
**B**ehavior and the data-source **L**ocation it would appear in. `queue_hunt`
parks a trigger (intel, a hunch, a fresh CVE, a past incident) on a prioritized
**backlog** so nothing is lost.

**2 · Hunt.** `open_hunt` starts a hypothesis-driven hunt. vala explores
read-only data sources and records each fact as an immutable Finding pointer, and
records intelligence (indicators, TTPs, actors) as first-class artifacts.

**3 · Conclude.** `store_hunt` writes the narrative page with a Confirmed /
Refuted / Inconclusive verdict — every declarative finding must cite a finding ID
or the page is rejected. A Refuted/Inconclusive verdict is a real result.

**4 · Automate — the deliverable.** A *confirmed* hunt's deliverable is a
detection. vala authors a Sigma rule for the proven behavior — study → author →
validate → test → document — editing one field at a time (preserving comments and
key order), validating against the official Sigma JSON schema offline, running
each rule's inline `tests:` through a built-in evaluation engine, and linking it
back to the hunt. It never forces a low-value rule onto a refuted hunt. vala ships
no detections of its own — point it at your directory with `detections_dir`
(default `detections`).

Everything is linked in Notion — backlog ↔ intel ↔ hunts ↔ detections — into one
graph, so coverage and gaps become measurable.

### Responding to alerts (secondary)

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

## Writing detections

Rules are [Sigma](https://sigmahq.io) YAML — the vendor-neutral
detection-as-code standard, portable across SIEM backends. A rule needs at least
`title`, `logsource`, and `detection`. vala rules also model two optional,
schema-valid custom fields:

- **`runbook:`** — inline response guidance (`triage`, `investigate`, `contain`,
  `escalate`, `references`) so a detection is respondable from the rule alone.
- **`tests:`** — `{name, event, match}` cases the evaluation engine runs, so a
  rule's logic is verifiable.

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

See the embedded reference rules under
[`internal/reference/sigma/`](internal/reference/sigma) for complete examples. The
offline evaluation engine (`internal/detect`) supports the common modifiers
(`contains`, `startswith`, `endswith`, `all`, `re`, `cidr`, `lt|lte|gt|gte`),
`*`/`?` wildcards, dotted field lookups, and the `1 of` / `all of` quantifiers.

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

`env` selects the policy environment (`dev`/`prod`). Notion database IDs enable
real Notion writes; leave them empty to run in local mode. Every non-read-only
tool call is gated by `--permission`: `ask` (default) prompts per call, `allow`
auto-approves for unattended runs, `deny` blocks all writes.

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
