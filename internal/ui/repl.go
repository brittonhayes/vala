// Package ui renders the interactive terminal experience: a Bubble Tea program
// with a scrolling transcript and an always-active input at the bottom, plus the
// permission prompt wired through the same event loop.
package ui

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/mcp"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/session"
	tea "github.com/charmbracelet/bubbletea"
)

// phaseThinking labels the spinner while waiting on the model between tool runs.
const phaseThinking = "Thinking"

// REPL drives an interactive agent session against a terminal.
type REPL struct {
	Agent   *agent.Agent
	Gate    *permission.Gate
	Session *session.Session
	Model   string

	// ContextWindow and AutoCompactThreshold drive optimistic auto-compaction:
	// when the latest prompt crosses ContextWindow*AutoCompactThreshold tokens,
	// the session compacts before continuing. A non-positive value disables it.
	ContextWindow        int64
	AutoCompactThreshold float64

	// Connect rebuilds a provider for the given provider id and model from the
	// latest stored credentials. /connect calls it to wire up or switch providers
	// mid-session. Nil disables /connect's live switching.
	Connect func(provider, model string) (llm.Provider, error)

	// Evidence reports how each configured MCP source connected, rendered in the
	// session banner so the operator sees what is available to hunt in (and any
	// source that failed) rather than having the failure swallowed by stderr.
	Evidence []mcp.EvidenceStatus

	styles  Styles
	program *tea.Program
}

// New builds a REPL. The Bubble Tea program reads from and writes to the
// controlling terminal directly.
func New(a *agent.Agent, gate *permission.Gate, sess *session.Session, model string, contextWindow int64, autoCompactThreshold float64) *REPL {
	return &REPL{
		Agent:                a,
		Gate:                 gate,
		Session:              sess,
		Model:                model,
		ContextWindow:        contextWindow,
		AutoCompactThreshold: autoCompactThreshold,
		styles:               DefaultStyles(),
	}
}

// Run starts the interactive program and blocks until the user quits.
func (r *REPL) Run(ctx context.Context) error {
	m := newChatModel(r)
	p := tea.NewProgram(m,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	r.program = p

	// Wire the permission gate into the event loop: the agent goroutine sends a
	// request and blocks on the reply channel until the operator answers in the UI.
	r.Gate.Prompt = func(name, summary string) bool {
		reply := make(chan bool, 1)
		p.Send(permMsg{name: name, summary: summary, reply: reply})
		return <-reply
	}

	_, err := p.Run()
	return err
}

// events forwards agent callbacks into the Bubble Tea program as messages so all
// transcript and session mutation happens on the UI goroutine.
func (r *REPL) events(p *tea.Program) agent.Events {
	return agent.Events{
		OnAssistantText: func(text string) { p.Send(assistantMsg(text)) },
		OnToolCall:      func(name, summary string) { p.Send(toolCallMsg{name: name, summary: summary}) },
		OnToolResult: func(name, content string, isErr bool) {
			p.Send(toolResultMsg{name: name, content: content, isErr: isErr})
		},
		OnPermissionDenied: func(name, summary string) {
			p.Send(deniedMsg{name: name})
		},
		OnUsage: func(input, output int64) { p.Send(usageMsg{input: input, output: output}) },
	}
}

// phaseFor maps a tool call to a present-tense spinner label that reads like the
// agent narrating its own work.
func phaseFor(name, summary string) string {
	switch name {
	case "bash":
		return "Running command"
	case "read":
		return "Reading " + filepath.Base(summary)
	case "write":
		return "Writing " + filepath.Base(summary)
	case "edit":
		return "Editing " + filepath.Base(summary)
	case "ls":
		return "Listing files"
	case "glob", "grep":
		return "Searching"
	case "validate_detection":
		return "Validating detection"
	case "ntn":
		return "Updating Notion"
	}
	return "Running " + name
}

// oneLine collapses whitespace and truncates s to n runes for compact display.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > n {
		return string([]rune(s)[:n]) + "…"
	}
	return s
}

// previewLines returns the first max lines of a tool result, trimmed, with an
// elision marker when the content is longer.
func previewLines(s string, max int) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= max {
		return lines
	}
	out := append([]string{}, lines[:max]...)
	return append(out, fmtMore(len(lines)-max))
}

func fmtMore(n int) string {
	if n == 1 {
		return "… (1 more line)"
	}
	return "… (" + itoa(n) + " more lines)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
