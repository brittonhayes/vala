---
name: sigma-authoring
description: The full checklist for authoring high-fidelity Sigma rules — logsource selection, tight conditions, falsepositive enumeration, ATT&CK tagging, runbooks, and executable tests.
---
# Authoring a high-fidelity Sigma rule

Use this checklist when writing or refining a Sigma detection. It expands on the
short guidance in the system prompt; work through it top to bottom for any
non-trivial rule.

## 1. Anchor on the behavior, not the indicator
- State the adversary behavior in one sentence before you touch YAML. A good
  detection fires on a *technique* (e.g. "disabling CloudTrail logging"), not a
  single brittle IOC.
- Find the closest gold-standard exemplar with `reference_detection` and mirror
  its shape — field names, condition style, runbook, and tests.

## 2. Pin the logsource precisely
- Set `logsource.product` (e.g. `aws`, `windows`), and `service`/`category` when
  they narrow the data (e.g. `service: cloudtrail`, `category: process_creation`).
- A loose logsource is the most common cause of a rule that "works" in test but
  never fires (or fires everywhere) in production. Match the field names your
  real telemetry uses.

## 3. Write the tightest condition that still catches the behavior
- Build named search identifiers (`selection`, `selection_api`, `filter_*`) and
  combine them in `condition` with `and`/`or`/`not`, `1 of selection_*`,
  `all of selection_*`.
- Use field modifiers deliberately: `|contains`, `|startswith`, `|endswith`,
  `|re`, `|cidr`, `|all`. Prefer an exact match over `|contains` when you can.
- Express benign carve-outs as explicit `filter_*` identifiers negated in the
  condition (`selection and not filter_known_good`) and leave a comment saying
  *why* each filter is safe. Never silently widen `selection` to dodge a false
  positive — that hides the carve-out from review.

## 4. Enumerate false positives honestly
- Populate `falsepositives` with the real benign sources of the behavior
  (break-glass procedures, backup jobs, blue-team tooling). An empty list on a
  medium/high rule is a red flag in review.
- If a false-positive source is common and separable, encode it as a `filter_*`;
  if it is rare, document it in `falsepositives` and let triage handle it.

## 5. Set metadata that makes the rule respondable and reviewable
- `title`: action-oriented and specific. `id`: a UUID v4. `status`:
  `experimental` until tested in the environment, then `test`, then `stable`.
- `level`: calibrate to response expectation — `critical`/`high` should page or
  be triaged fast; `low`/`informational` feed hunting and correlation.
- `tags`: map to MITRE ATT&CK (`attack.<tactic>` and `attack.t1234.001`).
  Accurate tags are what `update_coverage` and the coverage map rely on.
- `references`: link the technique and any report that motivated the rule.

## 6. Give it an inline runbook
- Add a `runbook:` with `triage`, `investigate`, `contain`, and `escalate`
  steps. Write them so an on-call responder who has never seen the rule can act:
  what to check first, which entities to pivot on, when to page.

## 7. Make it testable — and prove it
- Add `tests:` with at least one should-match case (`match: true`) and one
  should-not-match case (`match: false`). Design the negative case to exercise a
  realistic benign event your filters must let through, not a trivially unrelated
  one.
- Run `test_detection` and fix every failing case. A rule without a passing
  negative test has not been shown to avoid the obvious false positive.

## 8. Validate before you finish
- ALWAYS run `validate_detection` after writing or editing the rule. It runs the
  official Sigma schema check inside vala — do NOT shell out to `sigma-cli`,
  `yq`, or any external tool.
- Fix every reported issue. Aggregation/correlation conditions are not supported;
  if the behavior truly needs them, say so and propose a recurring hunt instead.

## Editing existing rules
- Prefer the field tools (`set_detection_meta`, `set_detection_logsource`,
  `edit_detection_logic`, `set_detection_runbook`, `manage_detection_tests`) over
  rewriting with `write`: they change one field, preserve comments and key order,
  and re-validate in one step.

## Definition of done
A rule is done when: logsource matches real telemetry, the condition is as tight
as the behavior allows with commented filters, `falsepositives` is populated, the
runbook is actionable, both a positive and negative test pass under
`test_detection`, and `validate_detection` is clean.
