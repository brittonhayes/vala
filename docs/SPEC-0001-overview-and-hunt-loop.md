# SPEC-0001 · Overview & the Hunt Loop

> vala is a single-binary, agentic threat hunter that runs one loop — Scope,
> Hunt, Conclude, Automate — and leaves a validated detection behind.

| Field | Value |
|---|---|
| **ID** | SPEC-0001 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `internal/agent/prompt.go`, `README.md`, `cmd/vala` |
| **Depends on** | — |

## 1. Purpose & scope

This is the root spec. It defines what vala **is**, the single loop it runs, and
the product principles every other spec inherits. It is the one place that
states the product's shape; the other specs detail the parts (the brain, the
tools, the detection engine, and so on).

It does **not** specify any component's internals — those live in their own
specs (see the [index](README.md)).

## 2. Definitions

- **vala** — the agentic threat-hunting system: one binary, one interactive
  agent, one loop.
- **Hunt loop** — the four-step spine: **Scope → Hunt → Conclude → Automate**.
- **Brain** — vala's persistence layer: a connected graph of backlog items,
  hunts, evidence, intel, and detections (see [SPEC-0002](SPEC-0002-brain-and-persistence.md)).
- **Hypothesis** — a falsifiable claim about a specific adversary behavior in a
  specific data source, the unit of work a hunt evaluates.
- **ABLE** — the discipline for phrasing a hypothesis: **A**ctor (optional),
  **B**ehavior (the testable TTP), **L**ocation (the data source), **E**vidence
  (what you'd expect to see).
- **Detection** — a [Sigma](https://sigmahq.io) rule: vendor-neutral
  detection-as-code, the deliverable of a confirmed hunt.
- **Verdict** — a hunt's terminal outcome: **Confirmed**, **Refuted**, or
  **Inconclusive**.

## 3. Requirements

### Product shape

- **R-0001-01** vala MUST ship as a single binary with no external detection
  toolchain: Sigma rules are validated and unit-tested offline, inside the
  binary (see [SPEC-0005](SPEC-0005-detection-engine.md)).
- **R-0001-02** vala MUST present **one** capability — hunting — not a menu of
  co-equal modes. Detection authoring MUST be framed as the *Automate* step of a
  hunt, never as a standalone mode or command.
- **R-0001-03** vala MUST NOT include an alert/incident-response runtime. It is a
  hunting tool; responding to alerts is out of scope (see §6).
- **R-0001-04** The agent MUST operate purely by composing tools — there are no
  modes or slash-commands that change agent behavior. The only commands are the
  REPL session commands of [SPEC-0010](SPEC-0010-cli.md).

### The loop

- **R-0001-05** vala MUST run a single loop with four steps in order: **Scope,
  Hunt, Conclude, Automate.** The system prompt MUST present the loop in this
  shape (`internal/agent/prompt.go`).
- **R-0001-06** **Scope** — a hypothesis SHOULD be phrased with ABLE, naming at
  least a **behavior** and a **data source**. Before opening new work the agent
  MUST recall the brain first; if a prior hunt already settled the hypothesis or
  a detection already covers the behavior, the agent MUST say so and stop rather
  than re-hunt settled ground.
- **R-0001-07** **Hunt** — investigation MUST be read-only. Every fact the agent
  relies on MUST be recorded as an immutable finding that returns a citable ID.
- **R-0001-08** **Conclude** — a hunt MUST close with exactly one verdict
  (Confirmed | Refuted | Inconclusive). A Refuted or Inconclusive verdict is a
  real, valid result that retires a hypothesis.
- **R-0001-09** **Automate** — the deliverable of a **Confirmed** hunt is a
  detection: the agent SHOULD author, validate, test, and link a Sigma rule for
  the proven behavior. A detection MUST NOT be forced onto a Refuted or
  Inconclusive hunt; a low-value rule is worse than none.
- **R-0001-10** Each step's work MUST be recorded in the brain as connected,
  first-class artifacts, so each hunt compounds on the last.

## 4. Behavior & interfaces

### The loop, end to end

```
            ┌─────────────────── the hunt loop ───────────────────┐
trigger ─►  Scope (ABLE)  ─►  Hunt (evidence)  ─►  Conclude  ─►  Automate
            backlog item      record findings      verdict        Sigma rule
                                                                   = deliverable
```

| Step | What happens | Primary tools | Brain writes |
|---|---|---|---|
| **Scope** | Phrase an ABLE hypothesis; recall prior work; optionally park a trigger on the backlog | `recall`, `queue_hunt` | Backlog |
| **Hunt** | Open the hunt; investigate read-only; record each fact and reusable intel | `open_hunt`, evidence tools, `record_finding`, `record_intel` | Hunts, Evidence, Intel |
| **Conclude** | Render the verdict; every declarative claim cites a finding ID | `store_hunt` | Hunts (closed), narrative page |
| **Automate** | On Confirmed, author + validate + test + link a Sigma rule | detection-authoring tools, `link_artifacts` | Detections, relations |

The tools are specified in [SPEC-0004](SPEC-0004-hunting-workflow.md) (hunting)
and [SPEC-0006](SPEC-0006-detection-authoring.md) (authoring). The brain graph
they write is specified in [SPEC-0002](SPEC-0002-brain-and-persistence.md).

### Why the loop ends in a detection (rationale)

The four reference threat-hunting frameworks converge on one conclusion: an
automated detection is the **output** of a successful hunt, not a separate
discipline. vala adopts this directly.

- **Sqrrl / Hunting Maturity Model** — the loop is explicitly designed to end in
  automation; maturity is graded largely by how much hunting a team has
  automated into detections.
- **Splunk PEAK** (Prepare, Execute, Act) — the *Act* phase's named deliverables
  include turning the hunt into a detection. PEAK scopes a hypothesis with ABLE.
- **TaHiTI** (Initiate, Hunt, Finalize) — *Finalize* recommends improvements
  including new detections, and feeds a first-class **backlog**.
- **Palantir ADS** — defines what "leave a detection behind" must contain to be
  any good: goal, ATT&CK categorization, false positives, validation, a
  response runbook. vala maps these onto Sigma fields it supports (MITRE tags,
  `falsepositives`, inline `runbook:`, inline `tests:`) — most of an ADS, as
  code.

This is why detection authoring is the *Automate* step (R-0001-02, R-0001-09)
and why the backlog is a first-class table (R-0001-06,
[SPEC-0002](SPEC-0002-brain-and-persistence.md)).

## 5. Acceptance criteria

- **A-0001-01** (R-0001-05) The system prompt built by `agent.SystemPrompt`
  enumerates the four steps Scope, Hunt, Conclude, Automate in order.
- **A-0001-02** (R-0001-02) No CLI subcommand, REPL command, or tool named
  "author detection" / "detection mode" exists; detection tools are reachable
  only as primitives the agent composes (`internal/tools/toolbox.go`).
- **A-0001-03** (R-0001-03) The tree contains no alert/case/response runtime
  package and no response tables in the brain schema (`brain.Schema()` lists
  exactly six stores: evidence, hunts, intel, detections, backlog, memory).
- **A-0001-04** (R-0001-01) `validate_detection` and `test_detection` run with
  no network access and no external binary (`internal/detect`); `go test ./internal/detect/...`
  passes offline.
- **A-0001-05** (R-0001-08) `store_hunt` accepts only `Confirmed`, `Refuted`, or
  `Inconclusive` as an outcome and writes that to the hunt's `status`.

## 6. Non-goals

- **No alert/incident response.** The governed alert/case loop has been removed.
- **No detection deployment.** vala leaves a validated, tested Sigma rule in the
  user's detections directory; deploying it to a SIEM is the user's pipeline
  (see [SPEC-0006](SPEC-0006-detection-authoring.md) §6).
- **No additional hunt types yet.** Only hypothesis-driven hunting is shipped;
  PEAK's baseline and model-assisted hunts are future work.
- **vala ships no detections of its own.** Reference rules under
  `internal/reference/sigma` are exemplars for the agent to learn shape from, not
  a content library to deploy.

## 7. Open questions

- Should PEAK's baseline (exploratory) and model-assisted hunt types become
  first-class, and if so do they need new verdict semantics?
- Should "tuning an existing detection" be a named deliverable of a
  Refuted/Inconclusive hunt, distinct from authoring a new one?

## 8. References

- [SPEC-0002](SPEC-0002-brain-and-persistence.md) — the brain the loop records into.
- [SPEC-0004](SPEC-0004-hunting-workflow.md) — the hunting tools.
- [SPEC-0006](SPEC-0006-detection-authoring.md) — the Automate-step authoring tools.
- [Introducing the PEAK Threat Hunting Framework — Splunk](https://www.splunk.com/en_us/blog/security/peak-threat-hunting-framework.html)
- [The Cyber Hunting Maturity Model — Sqrrl](https://medium.com/@sqrrldata/the-cyber-hunting-maturity-model-6d506faa8ad5)
- [TaHiTI: a threat hunting methodology (whitepaper)](https://www.betaalvereniging.nl/wp-content/uploads/TaHiTI-Threat-Hunting-Methodology-whitepaper.pdf)
- [Palantir Alerting & Detection Strategy Framework](https://github.com/palantir/alerting-detection-strategy-framework/blob/master/ADS-Framework.md)
