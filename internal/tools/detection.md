Validate Sigma detection rules against the official Sigma schema. This runs
entirely inside vala — no external CLI, no network — so use it freely
after authoring or editing any rule.

Validate a single file, or a whole directory:
- `{"path": "detections/aws_root_login.yml"}`
- `{"dir": "detections", "recursive": true}`

For each file it reports valid/invalid and, when invalid, the specific schema
violations (with the location in the rule). A Sigma rule requires at least
`title`, `logsource`, and `detection` (with a `condition`). Always validate a
rule before considering the task done.

Input (provide exactly one of path or dir):
- `path` (string): a single rule file to validate.
- `dir` (string): a directory of rule files (*.yml / *.yaml).
- `recursive` (boolean): with `dir`, also descend into subdirectories.
