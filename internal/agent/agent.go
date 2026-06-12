// Package agent runs the core conversation loop: it sends the conversation to
// the model, executes any tool calls the model requests (subject to the
// permission gate), feeds results back, and repeats until the model is done.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/mode"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/skills"
	"github.com/brittonhayes/vala/internal/tool"
)

// Events lets a caller (REPL, one-shot runner) observe the loop as it runs.
// Any field may be nil.
type Events struct {
	// OnAssistantText fires for each text block the model emits.
	OnAssistantText func(text string)
	// OnToolCall fires before a tool runs, with a one-line summary.
	OnToolCall func(name, summary string)
	// OnToolResult fires after a tool runs.
	OnToolResult func(name, content string, isError bool)
	// OnPermissionDenied fires when the gate blocks a call.
	OnPermissionDenied func(name, summary string)
	// OnUsage fires after each model response with the token counts for that
	// response. InputTokens covers the full prompt the model just saw (history,
	// tools, and system prompt), making it a good proxy for context fullness.
	OnUsage func(inputTokens, outputTokens int64)
}

func (e Events) assistantText(s string) {
	if e.OnAssistantText != nil {
		e.OnAssistantText(s)
	}
}
func (e Events) toolCall(name, summary string) {
	if e.OnToolCall != nil {
		e.OnToolCall(name, summary)
	}
}
func (e Events) toolResult(name, content string, isErr bool) {
	if e.OnToolResult != nil {
		e.OnToolResult(name, content, isErr)
	}
}
func (e Events) denied(name, summary string) {
	if e.OnPermissionDenied != nil {
		e.OnPermissionDenied(name, summary)
	}
}
func (e Events) usage(in, out int64) {
	if e.OnUsage != nil {
		e.OnUsage(in, out)
	}
}

// Session carries the per-session specialization the agent applies on top of the
// provider, registry, and gate: the active mode, the discovered skills it can
// load, and the names of the MCP evidence tools (which stay exposed in every
// mode regardless of the mode's tool policy).
type Session struct {
	Mode          mode.Mode
	Skills        *skills.Set
	EvidenceNames []string
}

// Agent ties together the model provider, tools, and permission gate.
type Agent struct {
	llm      llm.Provider
	registry *tool.Registry
	gate     *permission.Gate
	system   string
	maxSteps int

	// Mode state. workdir/maturity/opCtx/skills/evidence are retained so SetMode
	// can recompute the system prompt and the exposed tool set in place.
	mode       mode.Mode
	activeTool func(tool.Tool) bool // exposure filter derived from the mode
	workdir    string
	maturity   int
	opCtx      string
	skills     *skills.Set
	evidence   map[string]bool
}

// New constructs an Agent. workdir is used only to build the system prompt.
// operatorContext is the trusted standing context to embed (operator-authored
// VALA.md plus shared brain memories); the caller assembles it so the agent
// package stays free of the brain dependency. Empty means no context section.
// sess supplies the initial mode, the skill catalog, and the evidence tool names.
func New(client llm.Provider, registry *tool.Registry, gate *permission.Gate, workdir string, maxSteps, maturityLevel int, operatorContext string, sess Session) *Agent {
	if maxSteps <= 0 {
		maxSteps = 50
	}
	evidence := make(map[string]bool, len(sess.EvidenceNames))
	for _, n := range sess.EvidenceNames {
		evidence[n] = true
	}
	a := &Agent{
		llm:      client,
		registry: registry,
		gate:     gate,
		maxSteps: maxSteps,
		workdir:  workdir,
		maturity: maturityLevel,
		opCtx:    operatorContext,
		skills:   sess.Skills,
		evidence: evidence,
	}
	a.applyMode(sess.Mode)
	return a
}

// SetProvider swaps the model provider in place, so an operator can switch
// providers mid-session with /connect without losing the conversation. The
// system prompt and toolbox are unaffected.
func (a *Agent) SetProvider(p llm.Provider) { a.llm = p }

// SetMode swaps the active mode in place, so an operator can switch focus
// mid-session with /mode without losing the conversation. It recomputes the
// system prompt (mode body + active skills) and the exposed tool set. The
// provider, gate, registry, and history are unaffected.
func (a *Agent) SetMode(m mode.Mode) { a.applyMode(m) }

// Mode reports the active mode, for the UI banner and /mode confirmation.
func (a *Agent) Mode() mode.Mode { return a.mode }

// applyMode sets the active mode and recomputes the derived state: the tool
// exposure filter and the system prompt (which depends on the filtered tool
// names and the mode's bundled skills).
func (a *Agent) applyMode(m mode.Mode) {
	a.mode = m
	a.activeTool = a.modeFilter(m)
	a.system = SystemPrompt(m, mode.PromptInput{
		Workdir:       a.workdir,
		ToolNames:     a.exposedToolNames(),
		MaturityLevel: a.maturity,
	}, a.skills.ByIDs(m.Skills), a.opCtx)
}

// modeFilter derives the tool exposure predicate for a mode. The "skill" tool is
// exposed exactly when the mode bundles skills; MCP evidence tools are always
// exposed; otherwise the mode's policy decides (a nil policy exposes everything).
// The filter is never nil, so the agent always enforces it.
func (a *Agent) modeFilter(m mode.Mode) func(tool.Tool) bool {
	hasSkills := len(m.Skills) > 0
	policy := m.ToolPolicy
	return func(t tool.Tool) bool {
		switch {
		case t.Name() == "skill":
			return hasSkills
		case a.evidence[t.Name()]:
			return true
		case policy == nil:
			return true
		default:
			return policy(t)
		}
	}
}

// exposedToolNames lists the registered tool names the active filter exposes, in
// the registry's stable (name-sorted) order — the list shown in the prompt.
func (a *Agent) exposedToolNames() []string {
	names := make([]string, 0)
	for _, t := range a.registry.All() {
		if a.activeTool == nil || a.activeTool(t) {
			names = append(names, t.Name())
		}
	}
	return names
}

// Connected reports whether a model provider is wired up.
func (a *Agent) Connected() bool { return a.llm != nil }

// Run advances the conversation by one user turn. It appends the user input to
// history, drives the tool-use loop to completion, and returns the updated
// history so the caller can persist it and continue the session. Every tool is
// exposed and gated only by permission.Gate.Allow.
func (a *Agent) Run(ctx context.Context, history []llm.Message, userInput string, ev Events) ([]llm.Message, error) {
	if a.llm == nil {
		return history, fmt.Errorf("no model provider connected — run /connect to choose one")
	}
	messages := append(history, llm.UserText(userInput))
	decide := func(block llm.Block, summary string, t tool.Tool) (bool, string) {
		return a.gate.Allow(block.Name, summary, t.ReadOnly()), "Operator denied permission to run this tool."
	}
	return a.loop(ctx, messages, a.system, a.registry.ToolDefsFiltered(a.activeTool), decide, a.maxSteps, ev)
}

// decideFunc decides whether a tool call may run, returning the denial message
// to feed back to the model if not.
type decideFunc func(block llm.Block, summary string, t tool.Tool) (allow bool, denyMsg string)

// loop is the tool-use loop body. It runs until the model stops requesting
// tools or the step limit hits.
func (a *Agent) loop(ctx context.Context, messages []llm.Message, system string, tools []llm.ToolDef, decide decideFunc, maxSteps int, ev Events) ([]llm.Message, error) {
	for step := 0; step < maxSteps; step++ {
		resp, err := a.llm.Complete(ctx, system, messages, tools)
		if err != nil {
			return messages, fmt.Errorf("model request failed: %w", err)
		}
		messages = append(messages, llm.AssistantMessage(resp.Content...))
		ev.usage(resp.Usage.InputTokens, resp.Usage.OutputTokens)

		var toolResults []llm.Block
		for _, block := range resp.Content {
			switch block.Type {
			case llm.BlockText:
				if t := strings.TrimSpace(block.Text); t != "" {
					ev.assistantText(block.Text)
				}
			case llm.BlockToolUse:
				toolResults = append(toolResults, a.runToolUse(ctx, block, decide, ev))
			}
		}

		// No tools requested -> the model has produced its final answer.
		if len(toolResults) == 0 {
			return messages, nil
		}
		messages = append(messages, llm.UserMessage(toolResults...))
	}
	return messages, fmt.Errorf("reached step limit (%d) without completing", maxSteps)
}

// runToolUse executes a single tool_use block through the supplied decision
// function and returns the tool_result block to send back to the model.
func (a *Agent) runToolUse(ctx context.Context, block llm.Block, decide decideFunc, ev Events) llm.Block {
	summary := summarize(block.Name, block.Input)
	ev.toolCall(block.Name, summary)

	t, ok := a.registry.Get(block.Name)
	if !ok {
		msg := "unknown tool: " + block.Name
		ev.toolResult(block.Name, msg, true)
		return llm.ToolResultBlock(block.ID, msg, true)
	}

	// Enforce the mode's tool exposure: a tool hidden from the model in this mode
	// must not run even if the model names it directly.
	if a.activeTool != nil && !a.activeTool(t) {
		msg := "tool not available in " + a.mode.ID + " mode: " + block.Name
		ev.toolResult(block.Name, msg, true)
		return llm.ToolResultBlock(block.ID, msg, true)
	}

	if allow, denyMsg := decide(block, summary, t); !allow {
		ev.denied(block.Name, summary)
		// StopTurn-style: tell the model why so it can adapt instead of looping.
		return llm.ToolResultBlock(block.ID, denyMsg, true)
	}

	res, err := t.Run(ctx, block.Input)
	if err != nil {
		msg := "tool error: " + err.Error()
		ev.toolResult(block.Name, msg, true)
		return llm.ToolResultBlock(block.ID, msg, true)
	}
	ev.toolResult(block.Name, res.Content, res.IsError)
	return llm.ToolResultBlock(block.ID, res.Content, res.IsError)
}

// summarize renders a short, human-readable description of a tool call for
// permission prompts and UI, pulling the most relevant field per tool.
func summarize(name string, input json.RawMessage) string {
	var m map[string]any
	_ = json.Unmarshal(input, &m)
	get := func(k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	switch name {
	case "bash":
		return get("command")
	case "write", "edit", "read":
		return get("path")
	case "ntn":
		if args, ok := m["args"].([]any); ok {
			parts := make([]string, 0, len(args))
			for _, a := range args {
				parts = append(parts, fmt.Sprint(a))
			}
			return "ntn " + strings.Join(parts, " ")
		}
	case "grep", "glob":
		return get("pattern")
	case "recall":
		return get("query")
	}
	// Fallback (MCP and other tools we don't special-case): render the input
	// object as a compact, readable "key: value" list rather than raw JSON, so
	// the UI can show a glance of the query. Keys are sorted for stability.
	if len(m) > 0 {
		return compactArgs(m)
	}
	return strings.TrimSpace(string(input))
}

// compactArgs flattens a decoded JSON object into "k: v · k: v" with sorted
// keys. It is intentionally shallow — nested objects collapse to "{…}" — so the
// result stays a short, scannable summary.
func compactArgs(m map[string]any) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+compactVal(m[k]))
	}
	return strings.Join(parts, " · ")
}

// compactVal renders a single decoded JSON value compactly: arrays join with
// commas, nested objects collapse, and integer-valued floats lose the ".0".
func compactVal(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case nil:
		return "null"
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	case []any:
		els := make([]string, 0, len(t))
		for _, e := range t {
			els = append(els, compactVal(e))
		}
		return strings.Join(els, ",")
	case map[string]any:
		return "{…}"
	}
	return fmt.Sprint(v)
}
