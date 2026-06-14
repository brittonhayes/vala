package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/mode"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/tool"
	tea "github.com/charmbracelet/bubbletea"
)

// fakeProvider is a stand-in llm.Provider for exercising /connect without a
// network call.
type fakeProvider struct{}

func (fakeProvider) Complete(context.Context, string, []llm.Message, []llm.ToolDef) (*llm.Response, error) {
	return &llm.Response{}, nil
}
func (fakeProvider) Model() string        { return "fake-model" }
func (fakeProvider) Provider() string     { return "fakeprov" }
func (fakeProvider) ContextWindow() int64 { return 1000 }

func TestConnectSwitchesProviderLive(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newTestModel(t)
	m.repl.Agent = agent.New(nil, tool.NewRegistry(), permission.New(permission.ModeAsk, nil), "", 1, 1, "", agent.Session{Mode: mode.Default()})
	if m.repl.Agent.Connected() {
		t.Fatal("agent should start disconnected")
	}
	m.repl.Connect = func(provider, model string) (llm.Provider, error) { return fakeProvider{}, nil }

	res, _ := m.cmdConnect("anthropic")
	mm := res.(chatModel)
	if !mm.repl.Agent.Connected() {
		t.Fatal("agent should be connected after /connect")
	}
	if mm.repl.Model != "fakeprov · fake-model" {
		t.Errorf("banner model = %q, want fakeprov · fake-model", mm.repl.Model)
	}
}

func TestConnectUnknownProviderIsHandled(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m := newTestModel(t)
	// No panic even with a nil Agent: an unknown provider never reaches the swap.
	if _, _, handled := m.dispatchSlash("/connect nope-nope"); !handled {
		t.Fatal("/connect with unknown provider should be handled")
	}
}

func TestSplitCommand(t *testing.T) {
	tests := []struct {
		in       string
		wantName string
		wantArgs string
	}{
		{"help", "help", ""},
		{"compact", "compact", ""},
		{"compact focus on the auth hunt", "compact", "focus on the auth hunt"},
		{"  clear  ", "clear", ""},
		{"compact\textra", "compact", "extra"},
	}
	for _, tt := range tests {
		name, args := splitCommand(tt.in)
		if name != tt.wantName || args != tt.wantArgs {
			t.Errorf("splitCommand(%q) = (%q, %q), want (%q, %q)", tt.in, name, args, tt.wantName, tt.wantArgs)
		}
	}
}

func TestDispatchSlashHandling(t *testing.T) {
	// Keep /connect's provider listing hermetic — read from an empty temp config.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	tests := []struct {
		in          string
		wantHandled bool
	}{
		{"hunt for root logins", false}, // not a slash command
		{"/help", true},
		{"/connect", true},
		{"/clear", true},
		{"/compact", true},
		{"/bogus", true}, // unknown command is still "handled" (reports an error)
	}
	for _, tt := range tests {
		m := newTestModel(t)
		_, _, handled := m.dispatchSlash(tt.in)
		if handled != tt.wantHandled {
			t.Errorf("dispatchSlash(%q) handled = %v, want %v", tt.in, handled, tt.wantHandled)
		}
	}
}

func TestHelpUsesTransientCommandPanel(t *testing.T) {
	m := newTestModel(t)
	before := len(m.blocks)

	res, _ := m.cmdHelp("")
	m = res.(chatModel)

	if len(m.blocks) != before {
		t.Fatalf("/help appended transcript blocks: before=%d after=%d", before, len(m.blocks))
	}
	if !strings.Contains(m.commandPanel, "/help") || !strings.Contains(m.commandPanel, "/mode") {
		t.Fatalf("command panel missing help content: %q", m.commandPanel)
	}
}

func TestModeSwitchesSpecializationWithoutTranscript(t *testing.T) {
	m := newTestModel(t)
	m.repl.Agent = agent.New(nil, tool.NewRegistry(), permission.New(permission.ModeAsk, nil), "", 1, 1, "", agent.Session{Mode: mode.Default()})
	before := len(m.blocks)

	res, _ := m.cmdMode("detect")
	m = res.(chatModel)

	if got := m.repl.Agent.Mode().ID; got != "detect" {
		t.Fatalf("active mode = %q, want detect", got)
	}
	if len(m.blocks) != before {
		t.Fatalf("/mode detect appended transcript blocks: before=%d after=%d", before, len(m.blocks))
	}
	if m.commandPanel != "" {
		t.Fatalf("mode switch left command panel %q", m.commandPanel)
	}
	if footer := m.footer(); !strings.Contains(footer, "mode: detect") {
		t.Fatalf("footer should show active mode, got %q", footer)
	}
}

func TestModeCommandOpensInteractivePicker(t *testing.T) {
	m := newTestModel(t)
	m.repl.Agent = agent.New(nil, tool.NewRegistry(), permission.New(permission.ModeAsk, nil), "", 1, 1, "", agent.Session{Mode: mode.Default()})
	before := len(m.blocks)

	res, cmd := m.cmdMode("")
	m = res.(chatModel)

	if cmd == nil {
		t.Fatal("bare /mode should wait for an interactive picker response")
	}
	if m.choice == nil {
		t.Fatal("bare /mode should open a choice prompt")
	}
	if m.commandPanel != "" {
		t.Fatalf("bare /mode should not render a command panel, got %q", m.commandPanel)
	}
	view := m.choiceView()
	if !strings.Contains(view, "Switch mode") || !strings.Contains(view, "hunt") || !strings.Contains(view, "detect") {
		t.Fatalf("mode picker missing expected rows:\n%s", view)
	}

	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	res, _ = m.onChoiceKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("detect")})
	m = res.(chatModel)
	msg := (<-done).(modeChoiceMsg)
	res, _ = m.Update(msg)
	m = res.(chatModel)

	if got := m.repl.Agent.Mode().ID; got != "detect" {
		t.Fatalf("active mode = %q, want detect", got)
	}
	if len(m.blocks) != before {
		t.Fatalf("/mode picker appended transcript blocks: before=%d after=%d", before, len(m.blocks))
	}
	if m.choice != nil {
		t.Fatal("choice prompt should close after selecting mode")
	}
	if footer := m.footer(); !strings.Contains(footer, "mode: detect") {
		t.Fatalf("footer should show selected mode, got %q", footer)
	}
}

func TestClearResetsContext(t *testing.T) {
	m := newTestModel(t)
	m.history = []llm.Message{llm.UserText("hi")}
	m.lastInputTokens = 1234
	m.append("some transcript block")

	res, _ := m.cmdClear("")
	m = res.(chatModel)

	if len(m.history) != 0 {
		t.Errorf("history not cleared: len = %d", len(m.history))
	}
	if m.lastInputTokens != 0 {
		t.Errorf("lastInputTokens = %d, want 0", m.lastInputTokens)
	}
	// The banner survives; command status stays out of transcript scrollback.
	if len(m.blocks) != 1 {
		t.Fatalf("blocks = %d, want 1 (banner only)", len(m.blocks))
	}
}

func TestClearBusyIsNoOp(t *testing.T) {
	m := newTestModel(t)
	m.running = true
	m.history = []llm.Message{llm.UserText("hi")}

	res, _ := m.cmdClear("")
	m = res.(chatModel)

	if len(m.history) != 1 {
		t.Errorf("history cleared while running; len = %d, want 1", len(m.history))
	}
}

func TestShouldAutoCompact(t *testing.T) {
	// A history long enough to clear the minCompactGain guard.
	hist := []llm.Message{llm.UserText("a"), llm.UserText("b"), llm.UserText("c"), llm.UserText("d")}
	tests := []struct {
		name        string
		window      int64
		threshold   float64
		tokens      int64
		history     []llm.Message
		compactedAt int
		want        bool
	}{
		{"below threshold", 1000, 0.8, 700, hist, 0, false},
		{"at threshold", 1000, 0.8, 800, hist, 0, true},
		{"above threshold", 1000, 0.8, 900, hist, 0, true},
		{"disabled window", 0, 0.8, 900, hist, 0, false},
		{"disabled threshold", 1000, 0, 900, hist, 0, false},
		{"empty history", 1000, 0.8, 900, nil, 0, false},
		// Over budget but the history is still just the post-compaction seed plus
		// a turn or two: compaction can't help, so it must not fire (the loop guard).
		{"just compacted, no growth", 1000, 0.8, 900, hist, 4, false},
		{"just compacted, some growth", 1000, 0.8, 900, hist, 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newTestModel(t)
			m.repl.ContextWindow = tt.window
			m.repl.AutoCompactThreshold = tt.threshold
			m.lastInputTokens = tt.tokens
			m.history = tt.history
			m.compactedLen = tt.compactedAt
			if got := m.shouldAutoCompact(); got != tt.want {
				t.Errorf("shouldAutoCompact() = %v, want %v", got, tt.want)
			}
		})
	}
}
