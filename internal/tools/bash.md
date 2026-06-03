Run a shell command and return its combined stdout and stderr.

Use this for git, ripgrep, jq, the `aws` CLI, and any other investigative or
response action available on the host.

Guidance:
- To validate Sigma detection rules, use the `validate_detection` tool, NOT a
  shelled-out CLI (sigma-cli, yq, etc.).
- Prefer the dedicated `read`, `write`, `edit`, `ls`, `glob`, and `grep` tools
  for file work; they are safer and clearer than shelling out.
- Commands run via `sh -c` from the working directory. State (cwd, env) does
  NOT persist between calls — pass absolute paths and set env inline.
- Quote paths with spaces. Avoid destructive commands unless explicitly asked.
- Long output is truncated; pipe through `head`, `tail`, or `jq` to narrow it.

Input:
- `command` (string, required): the shell command to run.
- `timeout_s` (integer, optional): max seconds to run before the command is
  killed. Defaults to 120, capped at 600.
