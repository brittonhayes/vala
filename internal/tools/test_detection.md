Run a detection rule's inline `tests:` through vala's built-in Sigma
evaluation engine. Each test case is a sample event the engine matches against
the rule's `detection`/`condition`; the case's `match` field is the expected
outcome. This runs entirely in-process — no SIEM, no network.

Give every rule you author a `tests:` block with at least one should-match and
one should-NOT-match case, then run this tool and fix the rule until all cases
pass. A rule that is schema-valid but untested is not done.

```yaml
tests:
  - name: root console login fires
    event: { eventName: ConsoleLogin, userIdentity.type: Root }
    match: true
  - name: IAM user login is ignored
    event: { eventName: ConsoleLogin, userIdentity.type: IAMUser }
    match: false
```

`event` keys may be flat dotted paths (`userIdentity.type`) or nested maps. The
engine supports common Sigma modifiers (`contains`, `startswith`, `endswith`,
`all`, `re`, `cidr`, `lt|lte|gt|gte`) and `1 of`/`all of` quantifiers;
aggregation conditions (`| count() …`) are reported as unsupported rather than
silently passing.

Input:
- `path` (string, required): the rule file whose inline tests should run.
