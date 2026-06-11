Validate that the telemetry a hypothesis needs actually exists, and is complete
enough to test it, **before** you query. This is the Plan-&-validate-data stage
of the loop. Run it right after `open_hunt` and before any evidence query.

Inputs:
- `sources` — the data sources the hypothesis needs (e.g. `["cloudtrail", "guardduty"]`).
- `time_window` — the window the hunt covers (e.g. `"last 90 days"`).
- `completeness`, `retention` — what you know about coverage and whether the
  data still exists for the window you need.
- `validated` — `true` if the telemetry is present and complete enough.
- `gap` — if a check failed, the visibility gap: what is missing or incomplete.

Two outcomes, both recorded as immutable findings linked to the hunt:
- **Validated** → a `data_plan` finding. You may now query and `record_finding`.
- **Failed** (`validated:false` or a `gap`) → a `visibility_gap` finding. A
  failed check is a real, documented result — never a silent skip. When a hunt
  is blind, either pivot to a source you do have, or close it with
  `detection_tier: tier5_none_documented` and queue a forensic-readiness
  follow-up (`queue_hunt`) to get the missing telemetry stood up.

`store_hunt` rejects a hunt that recorded query findings without first
validating data (or recording a gap), so run this before you hunt.
