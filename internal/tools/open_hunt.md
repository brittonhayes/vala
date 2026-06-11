Open a hypothesis-driven threat hunt and make it the active hunt for this session.

Call this when you start hunting a threat question. It creates the hunt in the
brain and lets `record_finding` and `store_hunt` write to it. Pass the `question`
(required). Scope the hypothesis with ABLE: name the testable `behavior` and the
`data_source` (Location) you will hunt in — both are strongly recommended before
you start. If the hunt came off the backlog, pass its `backlog_id` so the item is
marked Opened and linked.

Pick a `hunt_type` (defaults to `hypothesis`):
- `hypothesis` — investigate a specific predicted behavior or TTP. Best when
  intel or a coverage gap points to a concrete technique.
- `baseline` — establish what normal looks like for a data source, then surface
  deviations. Best when the question is "what stands out here."
- `model_assisted` — reason over algorithmic leads (clustering, outliers,
  anomaly scores) you compute from the data you pull. Best for high-volume data
  where the leads are not obvious. vala has no built-in ML; this is a style of
  analysis over evidence-tool results, not a separate engine.

After opening: validate the telemetry you need with `validate_data` first, then
investigate with read-only tools (`scanner_execute_query` against the Scanner
data lake when connected, plus `read`, `grep`, `glob`), record each fact you rely
on with `record_finding` (cite the returned ID), and surface reusable intel with
`record_intel`. Close with `store_hunt`: a Confirmed / Refuted / Inconclusive
verdict **and** a detection-tier decision. Then update the coverage map with
`update_coverage` in feedback.
