# A simple, focused threat-hunting system for vala

## The question, answered first

> *Does detection development need to be an independent capability, or can it
> come as the deliverable outcome of threat hunting?*

**It is the deliverable.** Every mature threat-hunting team treats an automated
detection as the *output* of a successful hunt, not as a separate discipline
running in parallel. vala should stop presenting "Hunt threats" and "Author
detections" as two co-equal capabilities and present **one loop whose final act
is to leave a detection behind.** The Sigma-authoring machinery vala already has
is kept — but it is demoted from a *mode* to the *closing move of a hunt*.

The rest of this document shows why the top teams converge on this, and what the
smallest version of that system looks like on top of vala's existing brain.

---

## How the top teams actually hunt

Four reference frameworks dominate the field. They use different words and
arrive at the same loop.

**Sqrrl / David Bianco — The Hunting Loop & Hunting Maturity Model (HMM).** The
original and most-copied model. Four stages: *create a hypothesis → investigate
via tools & techniques → uncover new patterns/TTPs → **inform and enrich
analytics.*** The loop is explicitly designed to **end in automation**: "the goal
is to automate threats via an analytic so that your team can continue to focus on
the next new hunt." The Hunting Maturity Model (HMM0–HMM4) grades a team largely
by *how much of its successful hunting it has automated into detections* — HMM0
relies on alerts only; the higher levels turn hunts into a growing library of
automated analytics.

**Splunk PEAK (2023) — Prepare, Execute, Act with Knowledge.** The current
state-of-the-art reframe. Three phases:
- **Prepare** — pick the topic, research, scope the hypothesis.
- **Execute** — dive into the data and analyze.
- **Act** — *document, automate, communicate.*

The **Act phase's named deliverables include turning the hunt into a detection /
analytic.** PEAK recognizes three hunt *types* — hypothesis-driven, baseline
(exploratory data analysis), and model-assisted (M-ATH) — and scopes each hunt's
hypothesis with **ABLE**: **A**ctor (optional), **B**ehavior (the testable TTP —
the part that matters), **L**ocation (which data source), **E**vidence (what
you'd expect to see). A hypothesis that can't be phrased as ABLE isn't ready to
hunt.

**TaHiTI (Dutch financial sector) — Initiate, Hunt, Finalize.** The most
process-rigorous. *Initiate* turns a trigger (threat intel, a hunch, a past
incident) into a short abstract on a **hunting backlog**. *Hunt* refines that
abstract into an investigation plan (data sources, techniques, hypothesis) and
executes it. *Finalize* documents findings and, critically, **recommends
improvements — including new detections — and feeds the backlog.** The backlog is
a first-class object: hunts are queued, prioritized, and never lost.

**Palantir ADS — Alerting & Detection Strategy.** This is the *detection* side of
the handoff, and it tells you what "leave a detection behind" actually requires
to be any good. A detection isn't "a hunt confirmed X." An ADS documents: a
plain-language **Goal**, a **Categorization** (MITRE ATT&CK mapping), a
**Strategy Abstract**, **Technical Context**, **Blind Spots / known evasions**,
**False Positives**, a **Validation** procedure, **Priority**, and a **Response**
runbook. Principles: quality over quantity, peer review, continuous improvement.

### What they agree on

1. **Hunting is hypothesis-driven.** You don't "go look around." You state a
   falsifiable claim about a specific behavior in a specific data source, then
   confirm or refute it with evidence.
2. **The deliverable is automation.** "The key product of every hunt should be
   the creation or improvement of a detection." Formalizing the hunt→detection
   feedback loop is repeatedly named *the single biggest maturity factor* for a
   security program.
3. **A backlog drives the work.** Triggers become queued, prioritized hypotheses;
   nothing depends on a hunter remembering an idea.
4. **Everything is written down, linked, and reusable.** Hunts, the intel that
   triggered them, the evidence, and the detections they spawned form one graph —
   so coverage and gaps become measurable.

That graph is exactly what "Notion as the brain" is for.

---

## The insight that makes the system simple

vala originally advertised three co-equal jobs:

```
Hunt threats   │   Author detections   │   Respond to alerts
```

But the frameworks above describe **one** job with a tail:

```
            ┌─────────────────── the hunt loop ───────────────────┐
trigger ─►  scope (ABLE)  ─►  hunt (evidence)  ─►  conclude  ─►  automate
            backlog item      record findings      verdict        Sigma rule
                                                                   = the deliverable
```

"Author detections" is not a peer of "hunt" — it is the **last 10 minutes of a
confirmed hunt.** "Respond to alerts" is a different, reactive loop; it was once
a secondary surface in vala (a governed `open_case` runtime) but has since been
removed so the product is a focused hunting tool, not a SOC console.

So the simple, focused system is: **one hunt loop, with detection as its Act
phase, recorded end-to-end in Notion.**

---

## The vala Hunt Loop (the proposed system)

Four steps, each already a tool or a tiny extension of one. The names in
parentheses are vala tools that exist today.

### 1. Scope — turn a trigger into a hunting-backlog hypothesis
A trigger (threat intel, a hunch, a fresh CVE, a past incident) becomes a
**backlog row** with an ABLE-shaped hypothesis. This is the one genuinely new
piece: a lightweight **Hunt Backlog** so hypotheses are queued and prioritized
instead of started ad hoc.

- New: `queue_hunt(trigger, hypothesis, behavior, data_source, priority)` →
  writes a Backlog row (status `Queued`).
- `open_hunt` (exists) pulls a backlog item into an active hunt, or starts one
  directly. The system prompt should require the hypothesis to name a
  **behavior** and a **data source** before hunting — ABLE discipline, enforced
  in the prompt and lightly in the tool.

### 2. Hunt — investigate read-only, record every fact
Unchanged from today, and already excellent:
- `scanner_execute_query` / `scanner_load_context` (and other MCP evidence
  tools), `read`, `grep`, `glob` — read-only exploration.
- `record_finding` — each fact becomes an **immutable Evidence pointer** with an
  ID you must cite. (This is vala's strongest existing idea; it's the ADS
  "evidence" discipline made structural.)
- `record_intel` — indicators / TTPs / actors surfaced along the way become
  first-class, reusable artifacts.

### 3. Conclude — render a verdict, linted against evidence
Unchanged:
- `store_hunt` with a verdict — **Confirmed | Refuted | Inconclusive** — and a
  hunt page where every declarative claim must cite a finding ID or be marked a
  hypothesis (`LintHuntPage`). Refuted and Inconclusive are *successes*: they
  retire a hypothesis and update the backlog.

### 4. Automate — the deliverable
This is the reframed step. On a **Confirmed** hunt, the loop's natural and
expected next action is to **emit a detection** for the behavior that was proven:
- Author a Sigma rule with vala's existing field tools (`edit_detection_logic`,
  `set_detection_meta`, `set_detection_runbook`, `manage_detection_tests`),
  validate offline, and run its inline tests.
- The rule should carry the **ADS fields vala already supports or can map**:
  MITRE tags (Categorization), `falsepositives`, an inline `runbook:` (Response),
  and `tests:` (Validation). That is most of an ADS, *as code*.
- `link_artifacts` connects `hunt → detection` (and `intel → detection`) in the
  brain, so the hunt's deliverable is provably attached to the hunt.

A **Refuted/Inconclusive** hunt's deliverable is smaller but real: the retired
hypothesis, the evidence, and a backlog update (and sometimes a *tuning*
recommendation for an existing rule). No detection is forced where none is
warranted — forcing low-value rules is exactly the anti-pattern ADS warns
against ("quality over quantity").

---

## Notion as the brain: trim to what the loop needs

vala's brain is a tight five-table graph — exactly what the loop needs, and all
that remains now that the response tables (Alerts, Cases, Actions, Runs) are
gone:

| Database | Role in the loop | Status today |
|---|---|---|
| **Backlog** | Queued, prioritized ABLE hypotheses (the TaHiTI backlog) | **new — small** |
| **Hunts** | One row per hunt: question, hypothesis, verdict | exists |
| **Evidence** | Immutable finding pointers, linked to a hunt | exists (`record_finding`) |
| **Intel** | Indicators / TTPs / actors, reusable across hunts | exists (`record_intel`) |
| **Detections** | The graph node for each Sigma rule a hunt produced | exists (`RecordDetection`) |

The relations that make it a *brain* rather than a list:

```
Backlog ─►(opened as) Hunts ─►(produced) Detections
   ▲                    │ ▲                    ▲
   │                    ▼ │                    │
  Intel ───(triggered / surfaced / informed)──┘
            Evidence ──(backs)── Hunts
```

The four response tables (Alerts, Cases, Actions, Runs) and the `open_case`
runtime that used them have been **removed** — vala is a hunting tool, and the
graph above is its entire surface.

---

## So: independent capability, or deliverable?

**Deliverable — with the engineering rigor kept, not thrown away.**

- **Demote detection authoring from a mode to the Act phase of the hunt.** Don't
  advertise it as a third co-equal capability. A user should never run vala to
  "author a detection" in the abstract; they run it to *hunt*, and a confirmed
  hunt *ends* in a detection. This is the unanimous position of Sqrrl/HMM, PEAK,
  and TaHiTI.
- **Keep all the detection *machinery*.** The Sigma field-editors, offline schema
  validation, and the inline test engine are vala's implementation of ADS
  rigor (Validation, Blind Spots, False Positives, Response). Subordinating the
  capability does not mean weakening it — the Act phase is where that rigor pays
  off. A hunt that emits an unvalidated, untested, runbook-less rule has not
  actually delivered.
- **Don't force a detection on every hunt.** Refuted/Inconclusive hunts deliver a
  retired hypothesis and a backlog update. Forcing rules to exist would
  manufacture exactly the low-fidelity noise ADS exists to prevent.

The happy accident: **vala is already 90% of this.** The brain already models
`hunt → detection` as a relation (`internal/brain/intel.go`,
`link_artifacts`), and the README already says *"a hunt that confirms a TTP flows
straight into a detection."* This proposal is mostly a **reframing** — of the
system prompt, the README, and the product surface — plus one small new artifact
(the Backlog), not a rebuild.

---

## What was built

All four changes landed in this branch — a reframing plus one small new artifact,
no large refactor.

1. **Reframed the system prompt** (`internal/agent/prompt.go`). It now leads with
   the hunt loop (Scope → Hunt → Conclude → Automate) and presents detection
   authoring as the loop's *Automate* step, not a co-equal capability. ABLE is
   baked in: a hypothesis should name a *behavior* and a *data source*.
2. **Added the Hunt Backlog** — a sixth brain table (`internal/brain/backlog.go`,
   `DBBacklog`), the `queue_hunt` tool (`internal/tools/queue_hunt.go`), and
   `open_hunt` consuming a `backlog_id` to retire the item and link the hunt.
   `open_hunt`/the `Hunt` row also carry ABLE `behavior` + `data_source`.
3. **Made "automate" the expected close of a Confirmed hunt.** `store_hunt` now
   returns an outcome-aware directive: a `Confirmed` verdict drives straight into
   authoring + linking a Sigma rule (or an explicit "no detection warranted"
   note); a `Refuted`/`Inconclusive` verdict explicitly does not.
4. **Reframed the README** around the single loop. A later change removed the
   alert/response feature entirely (the `respond`/`governance`/`policy`
   packages, the case-response tools, and the four response tables), leaving a
   hunting-only product. This continues the project's existing narrowing
   (`7b80afd` reframing away from a detection-engineer persona, `a191a0e`
   "don't ship detections").

## What to *not* build (staying focused)

- No new hunt *types* yet. Ship hypothesis-driven well before adding PEAK's
  baseline or model-assisted hunts.
- No automated deployment of detections to a SIEM. vala leaves a validated,
  tested Sigma rule in the user's `detections_dir`; deploying it is the user's
  pipeline, not vala's job.
- No response runtime. The governed alert/case loop has been removed; vala is a
  focused hunting tool, and responding to alerts is out of scope.

---

## Sources

- [Introducing the PEAK Threat Hunting Framework — Splunk](https://www.splunk.com/en_us/blog/security/peak-threat-hunting-framework.html)
- [Hypothesis-Driven Hunting with the PEAK Framework — Splunk](https://www.splunk.com/en_us/blog/security/peak-hypothesis-driven-threat-hunting.html)
- [Model-Assisted Threat Hunting (M-ATH) with PEAK — Splunk](https://www.splunk.com/en_us/blog/security/peak-framework-math-model-assisted-threat-hunting.html)
- [PEAK security content — splunk/PEAK (GitHub)](https://github.com/splunk/PEAK)
- [The Threat Hunting Reference Model Part 1: The Hunting Maturity Model — Sqrrl](https://www.threathunting.net/files/The%20Threat%20Hunting%20Reference%20Model%20Part%201_%20Measuring%20Hunting%20Maturity%20_%20Sqrrl.pdf)
- [The Cyber Hunting Maturity Model — Sqrrl](https://medium.com/@sqrrldata/the-cyber-hunting-maturity-model-6d506faa8ad5)
- [A Framework for Cyber Threat Hunting — threathunting.net](https://www.threathunting.net/files/framework-for-threat-hunting-whitepaper.pdf)
- [TaHiTI: a threat hunting methodology (whitepaper)](https://www.betaalvereniging.nl/wp-content/uploads/TaHiTI-Threat-Hunting-Methodology-whitepaper.pdf)
- [Palantir Alerting & Detection Strategy Framework (GitHub)](https://github.com/palantir/alerting-detection-strategy-framework/blob/master/ADS-Framework.md)
- [Alerting and Detection Strategy Framework — Palantir Blog](https://blog.palantir.com/alerting-and-detection-strategy-framework-52dc33722df2)
- [Guarding the Gates: Detection Engineering and Threat Hunting — Intel 471](https://www.intel471.com/blog/guarding-the-gates-the-intricacies-of-detection-engineering-and-threat-hunting)
