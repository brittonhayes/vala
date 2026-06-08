Queue a trigger as a prioritized hunt hypothesis on the backlog — the Scope step
of the hunt loop.

Use this when a trigger (new threat intel, a hunch, a fresh CVE, a past incident)
should become a hunt but isn't being hunted right now. It records a durable,
rankable backlog item so hypotheses are never lost and the hunt program is
visible.

Scope the hypothesis with ABLE: name the testable adversary `behavior` and the
`data_source` (Location) it would appear in. Set a `priority`. When you start
hunting it, call `open_hunt` with the returned `backlog_id` so the item is marked
Opened and linked to the hunt.
