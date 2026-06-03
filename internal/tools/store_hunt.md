Store the threat hunt: write its narrative page and record its outcome.

Provide the `outcome` (Confirmed, Refuted, or Inconclusive) and structured
`findings`. Every finding must either cite one or more finding IDs (from
`record_finding`) or set `hypothesis: true`. The Evidence table is filled
automatically from what you recorded during the hunt.

If any finding is unsupported, the tool returns the offending findings and does
not store the hunt — fix them and call it again. Call this once, at the end of
the hunt, when you have gathered enough to evaluate the hypothesis.
