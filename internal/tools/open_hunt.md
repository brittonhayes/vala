Open a hypothesis-driven threat hunt and make it the active hunt for this session.

Call this when you start hunting a threat question. It creates the hunt in the
brain and lets `record_finding` and `store_hunt` write to it. Pass the `question`
(required). Scope the hypothesis with ABLE: name the testable `behavior` and the
`data_source` (Location) you will hunt in — both are strongly recommended before
you start. If the hunt came off the backlog, pass its `backlog_id` so the item is
marked Opened and linked.

After opening: investigate with read-only tools (`log_search`, `read`, `grep`,
`glob`), record each fact you rely on with `record_finding` (cite the returned
ID), surface reusable intel with `record_intel`, then call `store_hunt` once with
a Confirmed / Refuted / Inconclusive verdict. On a Confirmed hunt, the deliverable
is a detection: author a Sigma rule for the proven behavior and link it.
