Search the Notion brain for what is already known before opening new work.

`recall` reads back the artifacts vala has stored — prior `hunts`, `intel`,
`detections`, and the `backlog` — and returns the matches as compact lines. It
is the read counterpart to the tools that write the brain, and it is how each
hunt compounds on the last instead of repeating settled ground.

Run it at the start of the loop, before `open_hunt`:
- Has this behavior already been hunted? If a prior hunt settled the hypothesis,
  say so and stop rather than re-hunting it.
- Does a detection already cover the behavior? If so, the work may be done.
- What intel (indicators, TTPs, actors) already relates? Pull it forward instead
  of rediscovering it.

Inputs:
- `query` (required): free text to match — a behavior, MITRE technique, entity,
  or keyword. An empty query lists the most recent artifacts.
- `scope` (optional): `all` (default), `hunts`, `intel`, `detections`, or
  `backlog`.
- `limit` (optional): max results per scope (default 5).

Read-only: it never modifies the brain, so it needs no approval.
