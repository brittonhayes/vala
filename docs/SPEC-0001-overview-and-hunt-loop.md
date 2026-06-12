# SPEC-0001 · Overview & the Hunt Loop

> vala is a single-binary, agentic threat hunter that runs one eight-stage loop —
> scope, hypothesize, validate data, execute, deep-dive, decide, convert, feed
> back — and leaves a detection-tier decision (a validated Sigma rule or a
> documented coverage decision) behind every hunt.

| Field | Value |
|---|---|
| **ID** | SPEC-0001 |
| **Status** | Stable |
| **Updated** | 2026-06-10 |
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
- **Hunt loop** — the eight-stage spine: **scope & prioritize → form hypothesis
  → plan & validate data → execute & analyze → deep dive → document & decide →
  convert to detection → feed back**. Stages 4–6 iterate as evidence builds. The
  loop is prompt-driven, not a hard state machine.
- **Brain** — vala's persistence layer: a connected graph of backlog items,
  hunts, evidence, intel, detections, and coverage (see [SPEC-0002](SPEC-0002-brain-and-persistence.md)).
- **Hunt type** — the style a hunt runs: **hypothesis** (a specific predicted
  TTP), **baseline** (characterize normal, then surface deviations), or
  **model-assisted** (reason over algorithmic leads). Defaults to hypothesis.
- **Hypothesis** — a falsifiable claim about a specific adversary behavior in a
  specific data source, the unit of work a hunt evaluates.
- **ABLE** — the discipline for phrasing a hypothesis: **A**ctor (optional),
  **B**ehavior (the testable TTP), **L**ocation (the data source), **E**vidence
  (what you'd expect to see).
- **Data plan / visibility gap** — the result of the validate-data stage: a
  validated telemetry plan, or a recorded gap when the telemetry needed is
  missing or incomplete.
- **Detection-output tier** — the highest-fidelity output a hunt's finding
  supports, recorded with a rationale at close (the five-tier hierarchy in §4).
- **Detection** — a [Sigma](https://sigmahq.io) rule: vendor-neutral
  detection-as-code, the deliverable of detection tiers 1–2.
- **Coverage** — the durable, cross-hunt map of which ATT&CK techniques are
  covered, thinly covered, or uncovered, upserted by the feedback stage.
- **Verdict** — a hunt's terminal outcome: **Confirmed**, **Refuted**, or
  **Inconclusive**.
- **Maturity (HMM)** — the Hunting Maturity Model level (0–4) the harness runs
  at; an autonomy dial, not a behavioral mode (see [SPEC-0013](SPEC-0013-maturity-and-autonomy.md)).
- **Mode** — a selectable specialization the harness runs in: a system prompt, an
  exposed tool subset, and a set of bundled skills. **hunt** (this spec) is the
  default and reproduces the classic full loop; **detect** focuses on detection
  authoring. Modes are behavioral (they change what the agent does), distinct
  from maturity, which only tunes autonomy. See [SPEC-0014](SPEC-0014-modes-and-skills.md).

## 3. Requirements

### Product shape

- **R-0001-01** vala MUST ship as a single binary with no external detection
  toolchain: Sigma rules are validated and unit-tested offline, inside the
  binary (see [SPEC-0005](SPEC-0005-detection-engine.md)).
- **R-0001-02** **hunt** MUST be the default mode and the harness's center of
  gravity: within it, detection authoring is framed as the *convert* stage of a
  hunt (tiers 1–2 of the output hierarchy), not a co-equal activity. Additional
  modes (e.g. **detect**) are focused specializations layered on the same agent,
  toolbox, and brain, defined in [SPEC-0014](SPEC-0014-modes-and-skills.md) — not
  a menu of unrelated products.
  > Historical note: this requirement previously asserted vala presents *one*
  > capability with *no* modes. Modes were introduced intentionally (SPEC-0014);
  > hunt preserves the original behavior byte-for-byte as the default.
- **R-0001-03** vala MUST NOT include an alert/incident-response runtime. It is a
  hunting tool; responding to alerts is out of scope (see §6).
- **R-0001-04** **Within a mode**, the agent MUST operate purely by composing the
  tools that mode exposes — there are no per-turn sub-modes or behavior-changing
  slash-commands beyond mode selection itself. Mode selection (`/mode`, `--mode`,
  `VALA_MODE`, the `mode` config key) is the one behavioral switch; the only other
  commands are the REPL session commands of [SPEC-0010](SPEC-0010-cli.md). Modes
  are specified in [SPEC-0014](SPEC-0014-modes-and-skills.md).

### The loop

- **R-0001-05** vala MUST run a single loop with eight stages in order: **scope &
  prioritize, form hypothesis, plan & validate data, execute & analyze, deep
  dive, document & decide, convert to detection, feed back.** Stages 4–6 iterate.
  The system prompt MUST present the loop in this shape and order
  (`internal/agent/prompt.go`).
- **R-0001-06** **Scope & prioritize** — before opening new work the agent MUST
  recall the brain first; if a prior hunt already settled the hypothesis or a
  detection already covers the behavior, the agent MUST say so and stop rather
  than re-hunt settled ground. A trigger not hunted now SHOULD be parked on the
  backlog.
- **R-0001-07** **Form hypothesis** — a hypothesis SHOULD be phrased with ABLE,
  naming at least a **behavior** and a **data source**, and a **hunt type**
  (hypothesis | baseline | model_assisted) declared. The agent SHOULD reject a
  hypothesis it cannot test with available telemetry.
- **R-0001-08** **Plan & validate data** — before querying, the agent MUST
  validate that the telemetry the hypothesis needs exists and is complete enough.
  A passed check MUST be recorded as a data-plan finding; a failed check MUST be
  recorded as a visibility-gap finding — never a silent skip — and is itself an
  actionable outcome.
- **R-0001-09** **Execute, deep-dive & decide** — investigation MUST be
  read-only. Every fact the agent relies on MUST be recorded as an immutable
  finding that returns a citable ID. A hunt MUST close (`document & decide`) with
  exactly one verdict (Confirmed | Refuted | Inconclusive) **and** exactly one
  detection-output tier with a rationale. A Refuted or Inconclusive verdict is a
  real, valid result that retires a hypothesis.
- **R-0001-10** Each stage's work MUST be recorded in the brain as connected,
  first-class artifacts, so each hunt compounds on the last.
- **R-0001-11** **Convert to detection** — every hunt MUST record a
  detection-output tier decision (the highest-fidelity output the finding
  supports) with a rationale. Tiers 1–2 SHOULD produce a validated, tested Sigma
  rule for the proven behavior; tier 3 a recurring hunt; tier 4 a playbook; tier
  5 a justified no-build. A detection MUST NOT be forced where the finding does
  not support one; a low-value rule is worse than none. A tier-5 no-build MUST be
  justified, never silent.
- **R-0001-12** **Feed back** — a concluded hunt SHOULD update the coverage map
  for its technique and queue any follow-on hypotheses it surfaced, so coverage
  and the backlog compound across hunts.
- **R-0001-13** **Hypothesis weighting** — when scoping the next hunt, the agent
  SHOULD weight its choice in this order: detection coverage gaps first, then
  threat intel active against similar orgs, then what matters most given this
  environment's stack and assets.
- **R-0001-14** **Maturity tunes autonomy, not behavior** — the maturity level
  MUST only set the default permission mode and the prompt's gating framing (see
  [SPEC-0013](SPEC-0013-maturity-and-autonomy.md)). It MUST NOT add commands or
  tools, and MUST NOT itself act as a behavioral mode; within a given mode the
  loop and toolset are identical at every maturity level. (Behavioral
  specialization is the job of modes, R-0001-02 and [SPEC-0014](SPEC-0014-modes-and-skills.md);
  maturity is orthogonal to it.)

## 4. Behavior & interfaces

### The loop, end to end

```
        ┌──────────────────────── the hunt loop ────────────────────────┐
trigger ─► 1 Scope ─► 2 Hypothesis ─► 3 Validate data ─►┌─ 4 Execute ─┐
           recall      ABLE + type     data_plan/gap     │  5 Deep dive │ ⟲
           coverage    open_hunt       validate_data     └─ 6 Decide ──┘
                                                                │
                       8 Feed back ◄─ 7 Convert ◄───────────────┘
                       coverage map    tier decision
                       queue follow-on Sigma | recurring | playbook | no-build
```

| Stage | What happens | Primary tools | Brain writes |
|---|---|---|---|
| **1 Scope & prioritize** | Recall prior work + coverage; weight by gaps > intel > environment; park triggers | `recall` (incl. scope `coverage`), `queue_hunt` | Backlog |
| **2 Form hypothesis** | Phrase an ABLE hypothesis; pick a hunt type; open the hunt | `open_hunt` | Hunts (Open) |
| **3 Plan & validate data** | Confirm the needed telemetry exists; record a data plan or a visibility gap | `validate_data` | Evidence (`data_plan` / `visibility_gap`) |
| **4 Execute & analyze** | Investigate read-only; record each fact + reusable intel | evidence tools, `record_finding`, `record_intel` | Evidence, Intel |
| **5 Deep dive** | Triage, pivot, confirm or refute; preserve confidence | evidence tools, `record_finding` | Evidence |
| **6 Document & decide** | Render the verdict + a justified detection-tier decision; cite every claim | `store_hunt` | Hunts (closed), narrative page |
| **7 Convert to detection** | Build the highest-fidelity output the finding supports | detection-authoring tools, `link_artifacts` | Detections, relations |
| **8 Feed back** | Upsert the technique's coverage; queue follow-on hypotheses | `update_coverage`, `queue_hunt` | Coverage, Backlog |

The tools are specified in [SPEC-0004](SPEC-0004-hunting-workflow.md) (hunting)
and [SPEC-0006](SPEC-0006-detection-authoring.md) (authoring). The brain graph
they write is specified in [SPEC-0002](SPEC-0002-brain-and-persistence.md). The
coverage map, the feedback stage, and hypothesis weighting are specified in
[SPEC-0012](SPEC-0012-coverage-and-feedback.md); maturity in
[SPEC-0013](SPEC-0013-maturity-and-autonomy.md).

### The hierarchy of detection outputs

Not every hunt yields a clean rule. At `document & decide`, the agent picks the
**highest-fidelity** tier the finding supports and records why (R-0001-11):

| Tier | Constant | Output |
|---|---|---|
| 1 | `tier1_automated` | A production-grade, high-fidelity Sigma rule that fires reliably with low false positives. The preferred output. |
| 2 | `tier2_triage` | A lower-fidelity Sigma rule that surfaces candidates for review when behavior cannot be cleanly separated from benign activity. |
| 3 | `tier3_recurring_hunt` | Re-run the hunt query on a cadence when no rule is yet feasible. |
| 4 | `tier4_playbook` | Document the method and queries for future hunts when automation is premature. |
| 5 | `tier5_none_documented` | A justified no-build: the behavior is benign, out of scope, or blocked by a visibility gap (which becomes a forensic-readiness action). |

### Why the loop ends in a detection decision (rationale)

The four reference threat-hunting frameworks converge on one conclusion: a
durable detection output is the **outcome** of a successful hunt, not a separate
discipline. vala adopts this — and generalizes it: every hunt ends in a tier
decision, and the top two tiers produce a Sigma rule.

- **Sqrrl / Hunting Maturity Model** — the loop is explicitly designed to end in
  automation; maturity is graded largely by how much hunting a team has
  automated into detections. vala exposes this directly as a maturity dial (see
  [SPEC-0013](SPEC-0013-maturity-and-autonomy.md)).
- **Splunk PEAK** (Prepare, Execute, Act) — vala's eight stages are PEAK's
  spine. PEAK names three hunt types (hypothesis, baseline, model-assisted),
  scopes a hypothesis with ABLE, validates data before executing, and the *Act*
  phase's deliverables include turning the hunt into a detection.
- **TaHiTI** (Initiate, Hunt, Finalize) — *Finalize* recommends improvements
  including new detections, and feeds a first-class **backlog**.
- **Palantir ADS** — defines what "leave a detection behind" must contain to be
  any good: goal, ATT&CK categorization, false positives, validation, a
  response runbook. vala maps these onto Sigma fields it supports (MITRE tags,
  `falsepositives`, inline `runbook:`, inline `tests:`) — most of an ADS, as
  code.

This is why detection authoring is the *convert* stage for tiers 1–2 (R-0001-02,
R-0001-11), why the backlog and coverage map are first-class tables (R-0001-06,
R-0001-12, [SPEC-0002](SPEC-0002-brain-and-persistence.md)), and why a failed
data check is a recorded gap rather than a silent skip (R-0001-08).

## 5. Acceptance criteria

- **A-0001-01** (R-0001-05, R-0001-11) The system prompt built by
  `agent.SystemPrompt` enumerates the eight loop stages in order and the five
  detection-output tiers — `internal/agent/prompt_test.go`
  `TestSystemPromptEnumeratesLoopAndTiers`.
- **A-0001-02** (R-0001-02) No CLI subcommand, REPL command, or tool named
  "author detection" / "detection mode" exists; detection tools are reachable
  only as primitives the agent composes (`internal/tools/toolbox.go`).
- **A-0001-03** (R-0001-03) The tree contains no alert/case/response runtime
  package and no response tables in the brain schema (`brain.Schema()` lists
  exactly seven stores: evidence, hunts, intel, detections, backlog, memory,
  coverage).
- **A-0001-04** (R-0001-01) `validate_detection` and `test_detection` run with
  no network access and no external binary (`internal/detect`); `go test ./internal/detect/...`
  passes offline.
- **A-0001-05** (R-0001-09) `store_hunt` accepts only `Confirmed`, `Refuted`, or
  `Inconclusive` as an outcome and writes that to the hunt's `status`, and
  requires a `detection_tier`.
- **A-0001-06** (R-0001-08) A hunt that recorded query-kind evidence without a
  validated data plan and with no recorded gap is rejected at close
  (`internal/brain/hunt_test.go` `TestLintHuntRejectsQueryBeforeValidation`).
- **A-0001-07** (R-0001-14) The maturity level only changes the default
  permission mode and the prompt's `# Operating maturity` framing; no tool or
  command differs across levels (`internal/agent/prompt_test.go`
  `TestSystemPromptMaturityFraming`).

## 6. Non-goals

- **No alert/incident response.** The governed alert/case loop has been removed.
- **No detection deployment.** vala leaves a validated, tested Sigma rule in the
  user's detections directory; deploying it to a SIEM is the user's pipeline.
  Tier 1 is "production-grade," not "auto-deployed" — vala still ships nothing to
  a SIEM (see [SPEC-0006](SPEC-0006-detection-authoring.md) §6).
- **vala ships no detections of its own.** Reference rules under
  `internal/reference/sigma` are exemplars for the agent to learn shape from, not
  a content library to deploy.
- **No ML engine.** Model-assisted hunting is a style of analysis over
  evidence-tool results, not a built-in clustering/anomaly engine.

## 7. Open questions

- Should tier-3 recurring hunts be schedulable from within vala, or remain a
  documented cadence the operator runs?
- Should coverage status be derived automatically from linked detections rather
  than asserted by the agent at the feedback stage?

## 8. References

- [SPEC-0002](SPEC-0002-brain-and-persistence.md) — the brain the loop records into.
- [SPEC-0004](SPEC-0004-hunting-workflow.md) — the hunting tools.
- [SPEC-0006](SPEC-0006-detection-authoring.md) — the convert-stage authoring tools (tiers 1–2).
- [SPEC-0012](SPEC-0012-coverage-and-feedback.md) — the coverage map, feedback stage, and hypothesis weighting.
- [SPEC-0013](SPEC-0013-maturity-and-autonomy.md) — the maturity autonomy dial.
- [Introducing the PEAK Threat Hunting Framework — Splunk](https://www.splunk.com/en_us/blog/security/peak-threat-hunting-framework.html)
- [The Cyber Hunting Maturity Model — Sqrrl](https://medium.com/@sqrrldata/the-cyber-hunting-maturity-model-6d506faa8ad5)
- [TaHiTI: a threat hunting methodology (whitepaper)](https://www.betaalvereniging.nl/wp-content/uploads/TaHiTI-Threat-Hunting-Methodology-whitepaper.pdf)
- [Palantir Alerting & Detection Strategy Framework](https://github.com/palantir/alerting-detection-strategy-framework/blob/master/ADS-Framework.md)
