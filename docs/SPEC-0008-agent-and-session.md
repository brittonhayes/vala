# SPEC-0008 · Agent & Session

> The agent runs a bounded tool-use loop against Claude, records a transcript,
> and auto-compacts the conversation before it overflows the context window.

| Field | Value |
|---|---|
| **ID** | SPEC-0008 |
| **Status** | Stable |
| **Updated** | 2026-06-09 |
| **Source of truth** | `internal/agent/{agent,compact,prompt}.go`, `internal/session/session.go`, `internal/llm/client.go` |
| **Depends on** | SPEC-0001, SPEC-0003, SPEC-0009 |

## 1. Purpose & scope

This spec defines the agent runtime: the loop that calls the LLM and executes
tools, the system prompt's framing, the auto-compaction that keeps long sessions
within the context window, the transcript the session records, and the LLM
client that talks to Anthropic.

It does **not** define individual tools (SPEC-0003/0004/0006/0007), the
permission gate ([SPEC-0011](SPEC-0011-permissions-and-safety.md)), or config
loading ([SPEC-0009](SPEC-0009-configuration.md)).

## 2. Definitions

- **Loop** — one `Run`: append user input, then repeatedly call the model and
  execute requested tools until the model stops requesting tools.
- **Step** — one model call plus the execution of any tools it requested.
- **Compaction** — replacing the conversation history with a structured summary
  so work continues seamlessly past the context window.
- **Transcript / session** — the on-disk record of user/assistant/tool entries.

## 3. Requirements

### The loop

- **R-0008-01** `Run(input)` MUST append the user input to history, run the loop,
  and return the updated history for the caller to persist.
- **R-0008-02** Each step MUST call the LLM with the system prompt, the full
  history, and the (permission-filtered) tool set, then append the response to
  history.
- **R-0008-03** For each `tool_use` block, the agent MUST look up the tool,
  check the permission gate, run it on permit, and append a `tool_result` block
  (errors included) before the next step.
- **R-0008-04** The loop MUST terminate when a model response requests no tools
  (the model is done) and return the history.
- **R-0008-05** The loop MUST be bounded by `MaxSteps`; exceeding it MUST return
  a clear "reached step limit" error rather than looping forever.
- **R-0008-06** A denied tool call MUST return an error result that tells the
  model why, so it can adapt; it MUST NOT abort the run.
- **R-0008-07** The agent MUST emit observability events for at least: assistant
  text, tool call (with a one-line summary), tool result, permission denial, and
  token usage.

### System prompt

- **R-0008-08** The system prompt MUST frame vala as a hunting harness (not a
  persona), present the four-step loop in order, state the operating principles
  (investigate first, smallest change, adapt on denial, cite evidence), and
  declare tool outputs untrusted (see [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md),
  [SPEC-0011](SPEC-0011-permissions-and-safety.md)).

### Compaction

- **R-0008-09** When `ContextWindow > 0`, the agent MUST auto-compact when a
  turn approaches `AutoCompactThreshold × ContextWindow` tokens (default 0.80).
  Setting `ContextWindow` to 0 MUST disable auto-compaction.
- **R-0008-10** Compaction MUST produce a structured summary (request/intent,
  technical concepts, files/resources, errors/fixes, problem solving, pending
  tasks, current work, next step) and seed a fresh history with a continuation
  preamble plus that summary, so work resumes seamlessly.
- **R-0008-11** The user MUST be able to trigger compaction on demand with an
  optional focus that steers the summary (the `/compact` REPL command,
  [SPEC-0010](SPEC-0010-cli.md)).

### Session / LLM

- **R-0008-12** A session MUST append entries (`user` | `assistant` |
  `tool_call` | `tool_result`, with tool name and error flag) and persist them
  best-effort; a failed write MUST NOT abort the run. With no session directory,
  persistence is disabled silently.
- **R-0008-13** The LLM client MUST require an Anthropic API key (error if
  absent), default the model to `claude-opus-4-8`, cap response tokens at
  `MaxTokens` (default 8192), and call the Messages API non-streaming.

## 4. Behavior & interfaces

### Loop

```
Run(input):
  history += user(input)
  for step in 0..MaxSteps-1:
     resp = llm.Complete(system, history, tools)   # tools filtered by permission mode
     history += resp ; emit OnUsage
     results = []
     for block in resp:
        if text:      emit OnAssistantText
        if tool_use:  results += runToolUse(block)   # gate → Run → tool_result
     if results empty: return history                # model finished
     history += user(results)
  return error "reached step limit (MaxSteps)"
```

`runToolUse` emits `OnToolCall` (with `Summarize`'s one-liner), looks up the
tool, calls the permission `decide`, and on deny emits `OnPermissionDenied` and
returns an explanatory error result.

`Summarize(tool, input)` extracts a one-line summary for prompts: `command` for
`bash`; `path` for `read`/`write`/`edit`; `pattern` for `grep`/`glob`; `query`
for `recall`; `ntn <args>`; otherwise trimmed JSON.

### Compaction

`Compact(history, focus)` asks the model (no tools) to summarize using the
compaction system prompt, then `buildContinuationHistory` seeds a new history:

```
"This session is being continued from a previous conversation that ran out of
context. The conversation is summarized below. Continue the work seamlessly from
where it left off, using the summary as your source of truth for prior context."
```

followed by the structured summary. Returns `(newHistory, summaryText, error)`.

### Session & LLM

```go
type Entry struct { Time time.Time; Kind EntryKind; Tool string; Content string; IsError bool }
// EntryKind ∈ {user, assistant, tool_call, tool_result}
// Session.ID = "20060102-150405"; persisted to <dir>/<id>.json on each Add (best-effort)
// DefaultDir = ~/.local/share/vala/sessions (empty disables persistence)

llm.New(cfg) → error if cfg.APIKey == ""
llm.Complete(ctx, system, history, tools) → *anthropic.Message  // non-streaming
// model default: claude-opus-4-8 ; maxTokens default: 8192
```

Config knobs (`ContextWindow`, `AutoCompactThreshold`, `Model`, `MaxTokens`,
`MaxSteps`) are defined in [SPEC-0009](SPEC-0009-configuration.md).

## 5. Acceptance criteria

- **A-0008-01** (R-0008-04) A model response with no `tool_use` blocks ends the
  loop and returns history.
- **A-0008-02** (R-0008-05) A model that requests tools every step stops at
  `MaxSteps` with a "reached step limit" error.
- **A-0008-03** (R-0008-06) A denied call yields a `tool_result` with the denial
  reason and the loop continues.
- **A-0008-04** (R-0008-09) With `ContextWindow=0`, no compaction occurs; with a
  small window, a turn over threshold triggers compaction.
- **A-0008-05** (R-0008-10) After compaction the new history begins with the
  continuation preamble followed by the summary text.
- **A-0008-06** (R-0008-13) `llm.New` with no API key returns an error;
  otherwise `Model()` returns the configured id (default `claude-opus-4-8`).
- **A-0008-07** (R-0008-12) With a session dir, each `Add` writes a JSON file; a
  write failure does not error the run; with no dir, nothing is written.

## 6. Non-goals

- **No streaming UI protocol.** `Complete` returns a whole message; the TUI's
  rendering is out of scope here.
- **No multi-agent orchestration.** One agent, one loop.
- **No cross-session memory beyond the brain.** Durable memory is the brain
  ([SPEC-0002](SPEC-0002-brain-and-persistence.md)); the transcript is a record,
  not recalled context.

## 7. Open questions

- Should compaction be token-accurate (measured) rather than triggered on an
  estimated fraction of the window?
- Should `MaxSteps` and `MaxTokens` be env-overridable like the other knobs?

## 8. References

- [SPEC-0001](SPEC-0001-overview-and-hunt-loop.md) — the loop the prompt frames.
- [SPEC-0009](SPEC-0009-configuration.md) — the knobs referenced here.
- [SPEC-0011](SPEC-0011-permissions-and-safety.md) — the gate the loop consults.
