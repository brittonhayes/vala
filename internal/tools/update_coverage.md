Record or update the detection-coverage state for an ATT&CK technique. This is
the Feed-back stage of the loop: every hunt should end by updating the coverage
map so the next hunt is aimed where coverage is weakest and risk is highest.

It upserts a row keyed by `technique` (so re-hunting a technique updates its
state rather than duplicating it):
- `status` — `Covered` (a reliable detection fires), `Thin` (partial or
  low-fidelity), or `Uncovered` (no detection).
- `fidelity` — tie this to the hunt's detection tier: tier1 → `high`,
  tier2 → `medium`, tier3 → `low`, tier4/tier5 → `none`.
- `detections` — a short summary of what covers the technique, if anything.

Use `recall` with scope `coverage` at the start of a hunt to read the map back:
thin and uncovered techniques are the strongest hypotheses to pursue next.
