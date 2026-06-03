Search logs for events matching a query. Read-only.

Returns a JSON object with a stable `query_id`, the `count`, and the matching
`results`. Use `query_id` as the `pointer` when you record evidence, so every
claim traces back to a reproducible query. Use `*` to return all events.
