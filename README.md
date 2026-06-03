# vala

An agentic harness for **security detection & response (D&R)** engineering.

`vala` drives an LLM agent (Anthropic Claude) that investigates suspicious
activity, authors and validates [Sigma](https://sigmahq.io) detection rules,
runs shell/file tools, and documents its work in Notion via the `ntn` CLI. It
ships as a single static Go binary with **no external detection toolchain** —
Sigma rules are validated *and unit-tested* natively and offline, inside the
binary.

It has two modes:

- **Detection authoring** (`vala`, `vala run`) — study, author, validate, test,
  and document Sigma rules.
- **Incident response** (`vala respond`) — turn an alert into an auditable
  **case** in Notion under a code-enforced governance loop, proven safe over
  time by an adversarial regression harness (`vala harness`).

The agent is framed as a detection engineer and given a focused toolset for the
whole detection lifecycle: read the logs, study gold-standard exemplars, author
a rule field by field, **prove it with embedded test events**, and write up the
runbook.

> The architecture follows [charmbracelet/crush](https://github.com/charmbracelet/crush)
> (one `Tool` type + one embedded `.md` description per tool, a permission gate,
> sessions) and the "small extensible core" stance of
> [earendil-works/pi](https://github.com/earendil-works/pi).

## Install

```sh
go install github.com/brittonhayes/vala/cmd/vala@latest
export ANTHROPIC_API_KEY=sk-ant-...
```

Or build from source:

```sh
git clone https://github.com/brittonhayes/vala
cd vala
go build -o vala ./cmd/vala
```

## Usage

Interactive session (REPL):

```sh
vala
```

One-shot, non-interactive:

```sh
vala run "validate and test every rule in my detections directory, and report failures"
vala run --yes "author a Sigma rule for an attacker disabling GuardDuty: \
  study the reference rules first, add a runbook and two tests, then validate it"
```

Work an alert through the governed incident-response loop:

```sh
vala respond tests/ops/sample_alert.json
```

Replay the adversarial regression harness (no API key needed):

```sh
vala harness --fixtures tests
```

Flags: `--model <id>`, `--permission ask|allow|deny`.

## Incident response (`vala respond`)

`vala respond <alert.json>` drives an alert through a **phase-separated
governance loop** where every phase exposes the agent a smaller set of tools:

```
plan ─► evidence ─► propose ─► approval ─► execute ─► report
```

- **evidence** — read-only investigation. The agent searches logs and records
  every fact as an immutable **Evidence** pointer (a query ID, URL, or hash).
- **propose** — the agent proposes explicit **Actions** (citing evidence). It
  *cannot execute anything*: write/destructive tools are not even shown to it.
- **approval** — a human or policy approves each action. An approval binds to a
  single action by a deterministic `ActionID = hash(tool, canonical input)`.
- **execute** — only approved actions run, each at most once (idempotent).
- **report** — a narrative **case page** is written; every claim must cite an
  Evidence row or be marked a hypothesis, or the page is rejected.

This is enforced in **code**, not the prompt, at three points: the per-phase
tool-exposure filter, the authoritative `permission.Gate.Decide` backstop, and
the case-page evidence lint. Tool outputs are treated as untrusted data, so
return-channel prompt injection cannot reach a write tool during investigation.

The agent writes a structured **case brain** to Notion (via the `ntn` CLI):
**Alerts**, **Cases**, **Evidence**, **Actions**, and **Runs** databases plus
the narrative page. Without configured Notion database IDs the brain runs in
local mode and prints the case page to stdout. v1 ships two integrations: a mock
`log_search` evidence source and a gated `slack_notify` action.

### Policy

Governance is driven by editable YAML under [`policies/`](policies):

- `tools.yaml` — classifies each tool (`read`, `case_write`, `control`,
  `action_execute`) and lists per-environment (`dev`/`prod`) hard-deny rules.
  Unknown tools default to the most restricted class, so they **fail closed**.
- `decision.yaml` — which actions require approval, which auto-approve in `dev`,
  which must cite evidence, and the forbidden-behavior list.

### Harness (`vala harness`)

The harness replays adversarial scenario fixtures (`tests/`) through the real
governance machine in a deterministic recorded mode — **no LLM** — and scores
each on five safety dimensions: **approval compliance, no scope creep,
evidence-backed claims, injection resistance, schema validity**. It exits
non-zero on any failure or on a regression versus a committed baseline, so a
prompt or policy change that weakens behavior is caught in CI. The five
threat-model classes (injection, scope creep, evidence-less claims, schema
fuzzing, replay/idempotency) each have fixtures under `tests/`.

```sh
vala harness --fixtures tests --out report.json --baseline runner/baseline.json
```

## How it works

The agent works against a real workstation through a small set of tools. For
detection work the loop is: **study → author → validate → test → document.**

1. **Study.** `reference_detection` surfaces curated, gold-standard Sigma rules
   (adapted from [SigmaHQ](https://github.com/SigmaHQ/sigma)) embedded in the
   binary — each one carries an inline runbook and executable tests so the agent
   learns the shape of a respondable, review-proof rule before writing its own.
2. **Author.** Instead of rewriting whole YAML files, the agent edits one field
   at a time with the `set_detection_*` / `*_detection_*` tools. These preserve
   the rule's comments and key order and re-validate after every change.
3. **Validate.** `validate_detection` checks rules against the official Sigma
   JSON schema, embedded in the binary — no `sigma-cli`, `yq`, or Python.
4. **Test.** Rules carry inline `tests:` (sample events + expected outcome).
   `test_detection` runs them through a built-in Sigma evaluation engine, so a
   rule's *logic* is verified, not just its schema.
5. **Document.** `ntn` drives the official Notion CLI for runbooks and incident
   write-ups.

## Tools

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

Incident-response tools (used by `vala respond`, governed per phase):

| Tool | Class | Purpose |
|------|-------|---------|
| `log_search` | read | Query logs for evidence (mock-capable). |
| `record_evidence` | case_write | Append an immutable Evidence pointer. |
| `propose_action` | control | Propose a write action for approval (citing evidence). |
| `submit_for_approval` | control | End the proposal phase. |
| `write_case_page` | case_write | Write the narrative page (evidence-linted). |
| `slack_notify` | action_execute | The single gated write action in v1. |

The field-editing tools all funnel through one load → mutate → validate → write
pipeline: they change a single field, keep the file's comments intact, and
report only what changed plus the validation status — never the whole file.

## Detections

Detection rules are [Sigma](https://sigmahq.io) YAML files. **vala ships no
detections of its own** — you keep your rules wherever you already store them
and point vala at that directory with `detections_dir` (default
`detections`, relative to the working directory). Sigma is the vendor-neutral
detection-as-code standard; rules convert to many SIEM backends (and platforms
like [scanner.dev](https://scanner.dev) can ingest Sigma directly), so you write
once and stay portable.

A rule requires at least `title`, `logsource`, and `detection` (with a
`condition`). Vala rules also model two optional, schema-valid custom fields:

- **`runbook:`** — inline response guidance (`triage`, `investigate`, `contain`,
  `escalate`, `references`) so a detection is *respondable* from the rule alone.
- **`tests:`** — a list of `{name, event, match}` cases the evaluation engine
  runs, so a rule's logic is verifiable.

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

Validate and test your whole detections directory at once:

```sh
vala run "validate and test every rule in my detections directory"
```

### Built-in evaluation engine

The `test_detection` tool and the inline `tests:` field are backed by a
pragmatic, offline Sigma evaluation engine (`internal/detect`). It supports the
common modifiers (`contains`, `startswith`, `endswith`, `all`, `re`, `cidr`,
`lt|lte|gt|gte`), `*`/`?` wildcards, dotted/nested field lookups, and the
`1 of` / `all of` condition quantifiers. Unsupported constructs (e.g. aggregation
`| count() …` conditions) are reported as such rather than silently passing.

## Permissions

Every **non-read-only** tool call is gated.

- `ask` (default) — prompt the operator for each call. Answer `a` to allowlist a
  tool for the session.
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

`env` selects the policy environment (`dev`/`prod`). Notion database IDs enable
real Notion writes for `vala respond`; leave them empty to run the case brain in
local mode. Session transcripts are written to `~/.local/share/vala/sessions/`.

## Roadmap

The tool registry (`internal/tools/default.go`) is the single extension point.
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

CI (GitHub Actions) runs build, vet, `go test -race`, the adversarial harness
(diffed against `runner/baseline.json`), and a `gofmt` check on every push and
pull request.

## License

[MIT](LICENSE) © Britton Hayes
