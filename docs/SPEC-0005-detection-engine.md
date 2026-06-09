# SPEC-0005 · Detection Engine

> vala validates and unit-tests Sigma rules entirely offline, inside the binary —
> schema check, a condition grammar, field matching, and an inline test runner.

| Field | Value |
|---|---|
| **ID** | SPEC-0005 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `internal/detect/`, `internal/detect/sigma.schema.json` |
| **Depends on** | SPEC-0001 |

## 1. Purpose & scope

This spec defines the offline Sigma engine: how a rule is validated against the
Sigma schema, the subset of the condition grammar and field-match modifiers
supported, and how inline `tests:` are executed. It is the contract for "is this
rule valid and does its logic do what the author claims," with no network and no
external CLI.

It does **not** define the tools that call the engine or the surgical YAML
editing (that is [SPEC-0006](SPEC-0006-detection-authoring.md)).

## 2. Definitions

- **Rule** — a Sigma YAML document. A file MAY contain multiple documents
  separated by `---`.
- **Search identifier** — a named entry in `detection:` (a map, a list of maps,
  or a list of keyword strings).
- **Condition** — the boolean expression over search identifiers that decides
  whether a rule fires.
- **Modifier** — a `field|modifier` suffix changing how a value is matched
  (e.g. `CommandLine|contains`).
- **Inline test** — a `{name, event, match}` case under the custom `tests:`
  field, run by the engine to prove rule logic.

## 3. Requirements

### Offline & validation

- **R-0005-01** Validation MUST run fully offline and in-process: no network, no
  `sigma-cli`, `yq`, or other external binary.
- **R-0005-02** The engine MUST validate rules against the embedded official
  Sigma JSON schema, reporting issues per YAML document with a document index
  and a human-readable message.
- **R-0005-03** A rule MUST have `title`, `logsource`, and `detection` (with a
  `condition`). All other fields are optional but SHOULD follow the schema's
  enums when present (`status`, `level`, ATT&CK-shaped `tags`).
- **R-0005-04** Multi-document YAML files MUST be supported; each document is
  validated and testable independently.

### Condition grammar

- **R-0005-05** The engine MUST support: `and`, `or`, `not`, parentheses, the
  quantifiers `N of <pattern>` and `all of <pattern>` (with trailing-`*` prefix
  patterns and `them`). Keywords are case-insensitive; identifier names are
  case-sensitive.
- **R-0005-06** Aggregation conditions (e.g. `| count() > N`) MUST be rejected
  with an error, not silently ignored.

### Field matching

- **R-0005-07** The engine MUST support these modifiers: `contains`,
  `startswith`, `endswith`, `all`, `re`, `cidr`, `lt`, `lte`, `gt`, `gte`,
  `cased`. Matching MUST be case-insensitive by default; `cased` makes it
  case-sensitive.
- **R-0005-08** Default (modifier-less) matching MUST support Sigma glob
  wildcards `*` and `?` with backslash escaping; with no wildcard it is exact
  equality.
- **R-0005-09** Field lookup MUST resolve both flat dotted keys
  (`userIdentity.type`) and nested objects, including arrays of objects.
- **R-0005-10** List values MUST combine as OR by default and as AND under the
  `all` modifier. An absent field matches only an expected `null`.
- **R-0005-11** Unsupported encoding modifiers (`base64`, `base64offset`,
  `utf16*`, `wide`) MUST return an error rather than silently mis-matching.

### Inline tests

- **R-0005-12** The engine MUST run a rule's `tests:` list, each case
  `{name (string), event (object), match (bool)}`, evaluating the rule's
  condition against `event` and comparing the result to `match`.
- **R-0005-13** A test case passes iff evaluation produced no error and the
  result equals `match`. A rule's test run passes iff it has at least one case
  and all cases pass.
- **R-0005-14** A document carrying no `tests:` MUST be reported as such (flagged
  with an error on the result), not silently treated as passing.

## 4. Behavior & interfaces

### Schema

The official Sigma schema (Sigma V2.0.0) is embedded at
`internal/detect/sigma.schema.json` and compiled in-process. Required:
`title`, `logsource`, `detection.condition`. Recommended/enumerated:

- `id` — UUID v4
- `status` — `stable` | `test` | `experimental` | `deprecated` | `unsupported`
- `level` — `informational` | `low` | `medium` | `high` | `critical`
- `tags` — ATT&CK-shaped, e.g. `attack.t1078.004`
- `references`, `author`, `date`, `modified`, `falsepositives`, `fields`,
  `related`, `name`, `license`

The custom fields `runbook:` and `tests:` are permitted (additional properties);
they are defined in [SPEC-0006](SPEC-0006-detection-authoring.md).

### Condition grammar

```
expr       := or
or         := and ('or' and)*
and        := not ('and' not)*
not        := 'not' not | primary
primary    := '(' expr ')' | quantifier | NAME
quantifier := (INT | 'all') 'of' (PATTERN | 'them')
```

- `N of selection*` — true when at least N identifiers matching the prefix
  pattern are true; `all of selection*` requires every match.
- `N of them` / `all of them` — over all identifiers in the detection block.

### Search identifier matching

- **map** — every `field: value` entry must match (AND).
- **list of maps** — any map matches (OR); within a map, all entries match (AND).
- **list of keyword strings** — case-insensitive substring search across all
  stringified event values.

### Numeric & CIDR

- `lt|lte|gt|gte` coerce the actual value to a number; non-numeric actual values
  fail closed (no match), a non-numeric expected value is an error.
- `cidr` parses the expected value as a CIDR; non-IP actual values fail closed,
  an invalid CIDR is an error.

### Inline tests

```yaml
tests:
  - name: root console login fires
    event: { eventName: ConsoleLogin, userIdentity.type: Root }
    match: true
  - name: iam user login is ignored
    event: { eventName: ConsoleLogin, userIdentity.type: IAMUser }
    match: false
```

The runner returns, per case, `{Name, Want, Got, Err, Passed()}` and, per
document, `{Path, Doc, Title, Cases, Err, Passed()}`.

## 5. Acceptance criteria

- **A-0005-01** (R-0005-01) `go test ./internal/detect/...` passes with no
  network and no external binary on `PATH`.
- **A-0005-02** (R-0005-03) A rule missing `detection.condition` is reported
  invalid with the offending document index.
- **A-0005-03** (R-0005-05/06) `(a or b) and not c` parses and evaluates; a
  condition containing `| count()` returns an error.
- **A-0005-04** (R-0005-07/08) Cases exercising `contains`, `startswith`,
  `cidr`, `re`, and `*`/`?` wildcards evaluate as specified; `cased` flips
  case-sensitivity.
- **A-0005-05** (R-0005-11) A rule using `|base64` returns an explicit
  unsupported-modifier error.
- **A-0005-06** (R-0005-12/13) Every embedded reference rule's inline tests pass
  (`TestReferenceInlineTestsPass`), and every reference rule validates
  (`TestReferencesValid`).
- **A-0005-07** (R-0005-14) A document with no `tests:` yields a result whose
  `Err` is set.

## 6. Non-goals

- **No SIEM backend compilation.** The engine evaluates rules against sample
  events for testing; it does not convert Sigma to a vendor query language.
- **No aggregation/correlation.** Count/correlation rules are out of scope
  (R-0005-06).
- **No encoding transforms.** base64/utf16/wide matching is unsupported by
  design (R-0005-11).

## 7. Open questions

- Should a curated subset of encoding modifiers (`base64`, `base64offset`) be
  supported given how common they are in EDR rules?
- Should correlation/aggregation rules be supported as a separate, opt-in engine?

## 8. References

- [SPEC-0006](SPEC-0006-detection-authoring.md) — the tools that author and call the engine, and the `runbook:`/`tests:` shapes.
- [SigmaHQ specification](https://sigmahq.io)
- `internal/detect/{detect,condition,eval,runtest,schema}.go`
