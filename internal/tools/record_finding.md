Record a single finding from the current threat hunt.

A finding is an **immutable pointer** — a query ID/string, a URL, a file hash, or
a log reference — that backs a specific factual claim you discovered while
hunting. It is not free-form prose.

Use this during the explore phase for every fact you intend to state in the hunt
narrative. The tool returns a finding ID; you must cite that ID when you write up
the hunt's findings. Findings with no backing pointer are rejected when the hunt
page is stored.
