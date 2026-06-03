# vala

**An agentic security harness that hunts threats, builds detections, and works alerts — as a single binary.**

`vala` is a system, not a person: an agentic harness that orchestrates a
Notion-backed brain to investigate questions about threats. It **hunts** against a
hypothesis, stores the hunt and any **threat intelligence** it surfaces in Notion as
first-class artifacts, **connects** intel, hunts, alerts, and detections into one
graph, and feeds what it learns back into **detection development**. Hand it an alert
and it investigates, proposes actions, and writes up an auditable case — without ever
taking a destructive action you didn't approve.

Where a SIEM is something you search by hand, vala *explores*: it leans into
hypothesis-driven hunting, turns the results into a connected Notion brain, and
transforms that brain into detections.

It runs on Anthropic's Claude, ships as one static Go binary, and needs **no external
detection toolchain**: Sigma rules are validated *and* unit-tested natively and
offline, inside the binary. No `sigma-cli`, no `yq`, no Python.

## Why vala

Threat hunting and detection engineering are slow, manual, and easy to get wrong.
Hunting means exploring a question, chasing the data, and recording what you find so
it isn't lost. Writing a good Sigma rule means studying prior art, getting the
condition tight, proving it actually fires, and leaving behind a runbook so the next
person can respond. Working an alert means investigating carefully, acting reversibly,
and documenting every claim with evidence. `vala` does that exploratory work as a
harness — storing hunts and intelligence in Notion, connecting them to alerts and
detections — and bakes the safety rails into code, not a prompt, so it can be trusted
to do response work.

- **Hunting into a Notion-backed brain.** Point it at a threat question and it states
  a hypothesis, explores the data, records each finding with an evidence pointer, and
  stores the hunt — then connects intel, hunts, alerts, and detections into one graph.
- **Detection authoring, in the binary.** It reads logs, studies gold-standard
  exemplars, authors a rule field by field, proves it with embedded test events,
  and writes the runbook — and a hunt that confirms a TTP can be promoted straight
  into a detection.
- **Governed incident response.** Alerts flow through a phase-separated loop where
  the agent literally *cannot* execute a write action until a human or policy
  approves it. Enforced in code.
- **Provably safe over time.** An adversarial regression harness replays attack
  scenarios through the real governance machine in CI, so a prompt or policy change
  that weakens safety gets caught before it ships.

## Quickstart

```sh
go install github.com/brittonhayes/vala/cmd/vala@latest
export ANTHROPIC_API_KEY=sk-ant-...
```

Start an interactive session and ask it to do detection work:

```sh
vala
```

Or run a one-shot task non-interactively:

```sh
vala run "validate and test every rule in my detections directory, and report failures"

vala run --yes "author a Sigma rule for an attacker disabling GuardDuty: \
  study the reference rules first, add a runbook and two tests, then validate it"
```

Work an alert through the governed response loop:

```sh
vala respond tests/ops/sample_alert.json
```

Replay the adversarial safety harness (no API key needed):

```sh
vala harness --fixtures tests
```

Common flags: `--model <id>`, `--permission ask|allow|deny`.

> **Build from source:** `git clone https://github.com/brittonhayes/vala && cd vala
> && go build -o vala ./cmd/vala`

## What it does

### Author detections

`vala` (and `vala run`) put the agent to work on Sigma rules through a tight loop:
**study → author → validate → test → document.**

1. **Study.** `reference_detection` surfaces curated, gold-standard Sigma rules
   (adapted from [SigmaHQ](https://github.com/SigmaHQ/sigma)) embedded in the binary
   — each carries an inline runbook and executable tests, so the agent learns the
   shape of a respondable, review-proof rule before writing its own.
2. **Author.** Instead of rewriting whole YAML files, the agent edits one field at a
   time. Comments and key order are preserved, and the rule is re-validated after
   every change.
3. **Validate.** `validate_detection` checks rules against the official Sigma JSON
   schema, embedded in the binary — offline, no external tools.
4. **Test.** Rules carry inline `tests:` (sample events + expected outcome).
   `test_detection` runs them through a built-in Sigma evaluation engine, so a rule's
   *logic* is verified, not just its schema.
5. **Document.** The `ntn` tool drives the official Notion CLI for runbooks and
   write-ups.

You keep your rules wherever you already store them — **vala ships no detections of
its own.** Point it at your directory with `detections_dir` (default `detections`).

### Respond to alerts

`vala respond <alert.json>` drives an alert through a **phase-separated governance
loop** where each phase exposes the agent a smaller set of tools:

```
plan ─► evidence ─► propose ─► approval ─► execute ─► report
```

- **evidence** — read-only investigation. Every fact is recorded as an immutable
  **Evidence** pointer (a query ID, URL, or hash).
- **propose** — the agent proposes explicit **Actions**, citing evidence. It
  *cannot execute anything*: write/destructive tools aren't even shown to it.
- **approval** — a human or policy approves each action. An approval binds to a
  single action by a deterministic `ActionID = hash(tool, canonical input)`.
- **execute** — only approved actions run, each at most once (idempotent).
- **report** — a narrative **case page** is written; every claim must cite an
  Evidence row or be marked a hypothesis, or the page is rejected.

The result is an auditable **case brain** in Notion — **Alerts**, **Cases**,
**Evidence**, **Actions**, and **Runs** databases plus the narrative page. Without
configured Notion database IDs, the brain runs in local mode and prints the case
page to stdout. v1 ships a mock `log_search` evidence source and a gated
`slack_notify` action.

## How safety is enforced

The point of `vala respond` is that you can trust an autonomous agent with response
work. That trust comes from three code-level controls — **not** from asking the model
nicely in a prompt:

1. **Per-phase tool exposure.** Write tools don't exist for the agent during
   investigation, so it can't act early.
2. **The permission gate.** `permission.Gate.Decide` is the authoritative backstop:
   only approved actions run.
3. **Evidence lint.** The case page is rejected unless every claim cites evidence.

Because tool outputs are treated as untrusted data, return-channel **prompt injection
cannot reach a write tool** during investigation.

### Policy

Governance is driven by editable YAML under [`policies/`](policies):

- `tools.yaml` — classifies each tool (`read`, `case_write`, `control`,
  `action_execute`) and lists per-environment (`dev`/`prod`) hard-deny rules.
  Unknown tools default to the most restricted class, so they **fail closed**.
- `decision.yaml` — which actions require approval, which auto-approve in `dev`,
  which must cite evidence, and the forbidden-behavior list.

### The adversarial harness

`vala harness` replays adversarial scenario fixtures (`tests/`) through the real
governance machine in a deterministic recorded mode — **no LLM** — and scores each on
five safety dimensions: **approval compliance, no scope creep, evidence-backed
claims, injection resistance, schema validity**. It exits non-zero on any failure or
on a regression versus a committed baseline, so a prompt or policy change that
weakens behavior is caught in CI.

```sh
vala harness --fixtures tests --out report.json --baseline runner/baseline.json
```

The five threat-model classes (injection, scope creep, evidence-less claims, schema
fuzzing, replay/idempotency) each have fixtures under `tests/`.

## Writing detections

Detection rules are [Sigma](https://sigmahq.io) YAML files — the vendor-neutral
detection-as-code standard. Rules convert to many SIEM backends (and platforms like
[scanner.dev](https://scanner.dev) can ingest Sigma directly), so you write once and
stay portable.

A rule requires at least `title`, `logsource`, and `detection` (with a `condition`).
Vala rules also model two optional, schema-valid custom fields:

- **`runbook:`** — inline response guidance (`triage`, `investigate`, `contain`,
  `escalate`, `references`) so a detection is *respondable* from the rule alone.
- **`tests:`** — a list of `{name, event, match}` cases the evaluation engine runs,
  so a rule's logic is verifiable.

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

See the embedded gold-standard exemplars under
[`internal/reference/sigma/`](internal/reference/sigma) (surfaced at runtime by
`reference_detection`) for complete examples with runbooks and tests.

The inline `tests:` field is backed by a pragmatic, offline Sigma evaluation engine
(`internal/detect`). It supports the common modifiers (`contains`, `startswith`,
`endswith`, `all`, `re`, `cidr`, `lt|lte|gt|gte`), `*`/`?` wildcards, dotted/nested
field lookups, and the `1 of` / `all of` condition quantifiers. Unsupported
constructs (e.g. aggregation `| count() …` conditions) are reported as such rather
than silently passing.

## Tools

Detection-authoring tools:

| Tool | Read-only | Purpose |
|------|-----------|---------|
| `bash` | no | Run shell commands (git, jq, `aws`, …). |
| `read` / `write` / `edit` | read / no / no | File operations. |
| `ls` / `glob` / `grep` | yes | Navigate and search the workspace. |
| `reference_detection` | yes | Browse curated gold-standard Sigma exemplars. |
| `validate_detection` | yes | Validate Sigma rules against the embedded schema (offline). |
| `test_detection` | yes | Run a rule's inline `tests:` through the evaluation engine. |
| `set_detection_meta` | no | Set scalar metadata (title, id, status, level, …). |
| `set_detection_logsource` | no | Set the `logsource` block. |
| `edit_detection_logic` | no | Manage search identifiers and the `condition`. |
| `manage_detection_list` | no | Add/remove `references`, `falsepositives`, `tags`, `fields`. |
| `set_detection_runbook` | no | Set the inline response `runbook:`. |
| `manage_detection_tests` | no | Add/remove inline `tests:` cases. |
| `ntn` | no | Drive the official Notion CLI for runbooks & incident docs. |

The field-editing tools all funnel through one load → mutate → validate → write
pipeline: they change a single field, keep the file's comments intact, and report
only what changed plus the validation status — never the whole file.

Incident-response tools (used by `vala respond`, governed per phase):

| Tool | Class | Purpose |
|------|-------|---------|
| `log_search` | read | Query logs for evidence (mock-capable). |
| `record_evidence` | case_write | Append an immutable Evidence pointer. |
| `propose_action` | control | Propose a write action for approval (citing evidence). |
| `submit_for_approval` | control | End the proposal phase. |
| `write_case_page` | case_write | Write the narrative page (evidence-linted). |
| `slack_notify` | action_execute | The single gated write action in v1. |

## Permissions

Every **non-read-only** tool call is gated.

- `ask` (default) — prompt the operator for each call. Answer `a` to allowlist a tool
  for the session.
- `allow` — auto-approve (trusted, unattended runs; `vala run --yes`).
- `deny` — block all writes (investigation / dry-run only).

Read-only tools (`read`, `ls`, `glob`, `grep`, `reference_detection`,
`validate_detection`, `test_detection`) always run.

## Configuration

Settings layer (lowest priority first): built-in defaults →
`~/.config/vala/config.json` → `./.vala.json` → environment variables
(`ANTHROPIC_API_KEY`, `VALA_MODEL`, `VALA_PERMISSION`, `VALA_ENV`,
`SLACK_WEBHOOK_URL`).

```json
{
  "model": "claude-opus-4-8",
  "max_tokens": 8192,
  "permission": "ask",
  "allowlist": ["read", "ls", "glob", "grep"],
  "detections_dir": "detections",
  "max_steps": 50,
  "env": "dev",
  "notion": {
    "alerts": "", "cases": "", "evidence": "",
    "actions": "", "runs": "", "case_page_parent": ""
  }
}
```

`env` selects the policy environment (`dev`/`prod`). Notion database IDs enable real
Notion writes for `vala respond`; leave them empty to run the case brain in local
mode. Session transcripts are written to `~/.local/share/vala/sessions/`.

## Design

The architecture follows [charmbracelet/crush](https://github.com/charmbracelet/crush)
(one `Tool` type + one embedded `.md` description per tool, a permission gate,
sessions) and the "small extensible core" stance of
[earendil-works/pi](https://github.com/earendil-works/pi). The tool registry
(`internal/tools/default.go`) is the single extension point.

Planned next:

- An `aws` tool for cloud investigation & response (read verbs gated tighter).
- Sigma → backend query conversion (e.g. via pySigma) for export to a SIEM.
- Agent Skills (`SKILL.md`) for reusable D&R playbooks.

## Development

```sh
go build ./...
go vet ./...
go test ./...

# build / run the binary
go build -o vala ./cmd/vala
./vala version

# replay the adversarial harness
go run ./cmd/vala harness --fixtures tests
```

CI (GitHub Actions) runs build, vet, `go test -race`, the adversarial harness (diffed
against `runner/baseline.json`), and a `gofmt` check on every push and pull request.

## License

[MIT](LICENSE) © Britton Hayes
