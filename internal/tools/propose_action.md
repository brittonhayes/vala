Propose a write or destructive action for approval. This does NOT run the
action — it records an explicit proposal that a human or policy will approve or
deny before anything executes.

Provide the action `tool` name, the exact `input` it would run with, a
`rationale`, and the `evidence_ids` that justify it. Some actions require at
least one piece of evidence; the proposal is rejected otherwise.

Propose every action you intend to take, then call `submit_for_approval`. You
cannot execute actions yourself during investigation — that is by design.
