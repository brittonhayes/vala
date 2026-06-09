// Package agent runs the core conversation loop: it sends the conversation to
// the model, executes any tool calls the model requests (subject to the
// permission gate), feeds results back, and repeats until the model is done.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/permission"
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

// Agent ties together the model client, tools, and permission gate.
type Agent struct {
	llm      *llm.Client
	registry *tool.Registry
	gate     *permission.Gate
	system   string
	maxSteps int
}

// New constructs an Agent. workdir is used only to build the system prompt.
func New(client *llm.Client, registry *tool.Registry, gate *permission.Gate, workdir string, maxSteps int) *Agent {
	names := make([]string, 0)
	for _, t := range registry.All() {
		names = append(names, t.Name())
	}
	if maxSteps <= 0 {
		maxSteps = 50
	}
	return &Agent{
		llm:      client,
		registry: registry,
		gate:     gate,
		system:   SystemPrompt(workdir, names),
		maxSteps: maxSteps,
	}
}

// Run advances the conversation by one user turn. It appends the user input to
// history, drives the tool-use loop to completion, and returns the updated
// history so the caller can persist it and continue the session. Every tool is
// exposed and gated only by permission.Gate.Allow.
func (a *Agent) Run(ctx context.Context, history []anthropic.MessageParam, userInput string, ev Events) ([]anthropic.MessageParam, error) {
	messages := append(history, anthropic.NewUserMessage(anthropic.NewTextBlock(userInput)))
	decide := func(block anthropic.ContentBlockUnion, summary string, t tool.Tool) (bool, string) {
		return a.gate.Allow(block.Name, summary, t.ReadOnly()), "Operator denied permission to run this tool."
	}
	return a.loop(ctx, messages, a.system, a.registry.ToAnthropic(), decide, a.maxSteps, ev)
}

// decideFunc decides whether a tool call may run, returning the denial message
// to feed back to the model if not.
type decideFunc func(block anthropic.ContentBlockUnion, summary string, t tool.Tool) (allow bool, denyMsg string)

// loop is the tool-use loop body. It runs until the model stops requesting
// tools or the step limit hits.
func (a *Agent) loop(ctx context.Context, messages []anthropic.MessageParam, system string, tools []anthropic.ToolUnionParam, decide decideFunc, maxSteps int, ev Events) ([]anthropic.MessageParam, error) {
	for step := 0; step < maxSteps; step++ {
		resp, err := a.llm.Complete(ctx, system, messages, tools)
		if err != nil {
			return messages, fmt.Errorf("model request failed: %w", err)
		}
		messages = append(messages, resp.ToParam())
		ev.usage(resp.Usage.InputTokens, resp.Usage.OutputTokens)

		var toolResults []anthropic.ContentBlockParamUnion
		for _, block := range resp.Content {
			switch block.Type {
			case "text":
				if t := strings.TrimSpace(block.Text); t != "" {
					ev.assistantText(block.Text)
				}
			case "tool_use":
				toolResults = append(toolResults, a.runToolUse(ctx, block, decide, ev))
			}
		}

		// No tools requested -> the model has produced its final answer.
		if len(toolResults) == 0 {
			return messages, nil
		}
		messages = append(messages, anthropic.NewUserMessage(toolResults...))
	}
	return messages, fmt.Errorf("reached step limit (%d) without completing", maxSteps)
}

// runToolUse executes a single tool_use block through the supplied decision
// function and returns the tool_result block to send back to the model.
func (a *Agent) runToolUse(ctx context.Context, block anthropic.ContentBlockUnion, decide decideFunc, ev Events) anthropic.ContentBlockParamUnion {
	summary := summarize(block.Name, block.Input)
	ev.toolCall(block.Name, summary)

	t, ok := a.registry.Get(block.Name)
	if !ok {
		msg := "unknown tool: " + block.Name
		ev.toolResult(block.Name, msg, true)
		return anthropic.NewToolResultBlock(block.ID, msg, true)
	}

	if allow, denyMsg := decide(block, summary, t); !allow {
		ev.denied(block.Name, summary)
		// StopTurn-style: tell the model why so it can adapt instead of looping.
		return anthropic.NewToolResultBlock(block.ID, denyMsg, true)
	}

	res, err := t.Run(ctx, block.Input)
	if err != nil {
		msg := "tool error: " + err.Error()
		ev.toolResult(block.Name, msg, true)
		return anthropic.NewToolResultBlock(block.ID, msg, true)
	}
	ev.toolResult(block.Name, res.Content, res.IsError)
	return anthropic.NewToolResultBlock(block.ID, res.Content, res.IsError)
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
	return strings.TrimSpace(string(input))
}
