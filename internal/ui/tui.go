package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/session"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// spinnerFrames is a smooth braille cycle that reads as continuous motion.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// maxInputLines caps how tall the input box grows as text wraps before it
// starts to scroll internally, mirroring Claude Code's expanding composer.
const maxInputLines = 10

// --- messages forwarded from the agent goroutine into the event loop ---

type assistantMsg string
type toolCallMsg struct{ name, summary string }
type toolResultMsg struct {
	name, content string
	isErr         bool
}
type deniedMsg struct{ name string }
type turnDoneMsg struct {
	history []llm.Message
	err     error
}

// usageMsg carries token counts reported by the agent after each model response.
type usageMsg struct{ input, output int64 }

// compactDoneMsg reports the result of a /compact or auto-compaction run.
type compactDoneMsg struct {
	history []llm.Message
	summary string
	auto    bool // true when triggered by auto-compaction rather than /compact
	err     error
}

// permMsg asks the operator to approve a tool call. The agent goroutine blocks
// on reply until the UI answers.
type permMsg struct {
	name, summary string
	reply         chan bool
}

// chatModel is the Bubble Tea model: a scrolling transcript with an always-on
// input box at the bottom that accepts new messages even while the agent runs.
type chatModel struct {
	repl   *REPL
	styles Styles
	md     *markdownRenderer

	vp viewport.Model
	ta textarea.Model
	sp spinner.Model

	width, height int
	ready         bool

	blocks  []string      // rendered transcript blocks, top to bottom
	history []llm.Message // conversation carried across turns
	queue   []string      // messages typed while a turn is running

	running    bool
	compacting bool // a compaction LLM call is in flight
	phase      string
	started    time.Time
	cancel     context.CancelFunc

	lastInputTokens   int64 // most recent prompt token count, for auto-compaction
	compactedLen      int   // history length right after the last compaction (its seed)
	warnedTightBudget bool  // true once we've warned that overhead alone fills the window

	perm *permMsg // pending permission request, nil when none

	// Slash-command autocomplete: when the input is a bare "/foo" (no args yet),
	// compMatches holds the fuzzy-ranked commands and compIdx the highlighted row.
	compActive  bool
	compMatches []slashCommand
	compIdx     int
}

// maxCompletionRows caps how many command suggestions show at once so the menu
// never crowds out the transcript.
const maxCompletionRows = 8

func newChatModel(r *REPL) chatModel {
	ta := textarea.New()
	ta.Placeholder = "Ask vala to investigate, write a detection, or run a command…"
	ta.Prompt = "› "
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.MaxHeight = maxInputLines
	ta.SetHeight(1)
	// Keep the input visually flat; the surrounding box provides the frame.
	ta.FocusedStyle.Base = lipgloss.NewStyle()
	ta.BlurredStyle.Base = lipgloss.NewStyle()
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Prompt = r.styles.Prompt
	ta.BlurredStyle.Prompt = r.styles.Hint
	ta.FocusedStyle.Placeholder = r.styles.Hint
	ta.BlurredStyle.Placeholder = r.styles.Hint
	// Enter submits; explicit newlines come from ctrl+j or alt+enter.
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("ctrl+j", "alt+enter"))
	ta.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Spinner{Frames: spinnerFrames, FPS: time.Second / 10}
	sp.Style = r.styles.Spinner

	m := chatModel{
		repl:   r,
		styles: r.styles,
		md:     newMarkdownRenderer(80),
		ta:     ta,
		sp:     sp,
		phase:  phaseThinking,
	}
	m.blocks = append(m.blocks, m.banner())
	return m
}

func (m chatModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg.Width, msg.Height)

	case tea.KeyMsg:
		return m.onKey(msg)

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd

	case assistantMsg:
		m.phase = phaseThinking
		m.repl.Session.Add(session.Entry{Kind: session.KindAssistant, Content: string(msg)})
		m.append("\n" + m.md.render(string(msg)))
		return m, nil

	case toolCallMsg:
		m.phase = phaseFor(msg.name, msg.summary)
		m.repl.Session.Add(session.Entry{Kind: session.KindToolCall, Tool: msg.name, Content: msg.summary})
		// Routine tool activity stays grayscale and terse, like Claude Code: a dim
		// bullet, a muted name, and a clean argument only when there is one worth
		// showing. Raw JSON blobs are suppressed inline (they live in the
		// transcript); only the agent's own text earns color.
		line := "  " + m.styles.ToolMeta.Render("⏺") + " " + m.styles.Result.Render(msg.name)
		if s := oneLine(msg.summary, 64); s != "" {
			line += "  " + m.styles.Hint.Render(s)
		}
		// Lead with a blank line so each tool group is set off from the surrounding
		// text and from the prior group; the result line stays tucked beneath.
		m.append("\n" + line)
		return m, nil

	case toolResultMsg:
		m.phase = phaseThinking
		m.repl.Session.Add(session.Entry{Kind: session.KindToolResult, Tool: msg.name, Content: msg.content, IsError: msg.isErr})
		m.append(m.renderResult(msg.content, msg.isErr))
		return m, nil

	case deniedMsg:
		m.append("  " + m.styles.Denied.Render("✗ denied") + "  " + m.styles.ToolMeta.Render(msg.name))
		return m, nil

	case permMsg:
		m.perm = &msg
		m.ta.Blur()
		return m, nil

	case turnDoneMsg:
		return m.onTurnDone(msg)

	case usageMsg:
		m.lastInputTokens = msg.input
		return m, nil

	case compactDoneMsg:
		return m.onCompactDone(msg)

	case spinner.TickMsg:
		if !m.running {
			return m, nil
		}
		var cmd tea.Cmd
		m.sp, cmd = m.sp.Update(msg)
		return m, cmd
	}

	// Default: feed the input box (typing, cursor blink, etc.).
	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	return m, cmd
}

// onKey routes key presses, accounting for the permission modal and the
// running/idle distinction.
func (m chatModel) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Permission modal captures the keyboard until answered.
	if m.perm != nil {
		switch strings.ToLower(msg.String()) {
		case "y", "enter":
			m.answerPerm(true, false)
		case "a":
			m.answerPerm(true, true)
		case "n", "esc", "ctrl+c":
			m.answerPerm(false, false)
		}
		return m, nil
	}

	// The autocomplete menu captures navigation and selection keys while it is
	// open; everything else (typing, backspace) falls through to the textarea and
	// re-filters the menu below.
	if m.compActive {
		switch msg.String() {
		case "up", "ctrl+p":
			if m.compIdx > 0 {
				m.compIdx--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.compIdx < len(m.compMatches)-1 {
				m.compIdx++
			}
			return m, nil
		case "tab":
			// Fill in the highlighted command and keep editing so args can follow.
			m.acceptCompletion()
			m.relayout()
			return m, nil
		case "enter":
			// Run the highlighted command immediately.
			m.acceptCompletion()
			return m.submit()
		case "esc":
			m.compActive = false
			m.relayout()
			return m, nil
		}
	}

	switch msg.String() {
	case "ctrl+c":
		if m.running {
			m.interrupt()
			return m, nil
		}
		return m, tea.Quit
	case "esc":
		if m.running {
			m.interrupt()
		}
		return m, nil
	case "enter":
		return m.submit()
	case "shift+tab":
		// Cycle the permission disposition (ask → allow → deny) without leaving
		// the session. The footer reflects the new mode immediately.
		m.repl.Gate.CycleMode()
		return m, nil
	case "pgup", "pgdown", "ctrl+u", "ctrl+d":
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	m.updateCompletion()
	m.relayout()
	return m, cmd
}

// updateCompletion recomputes the autocomplete menu from the current input. The
// menu is shown only while the operator is typing a bare slash-command name —
// input that starts with "/" and has no whitespace yet — so it disappears the
// moment they start typing arguments or anything that is not a command.
func (m *chatModel) updateCompletion() {
	val := m.ta.Value()
	if !strings.HasPrefix(val, "/") || strings.ContainsAny(val, " \t\n") {
		m.compActive = false
		m.compMatches = nil
		m.compIdx = 0
		return
	}
	matches := matchCommands(m.commands(), val[1:])
	if len(matches) == 0 {
		m.compActive = false
		m.compMatches = nil
		m.compIdx = 0
		return
	}
	m.compMatches = matches
	m.compActive = true
	if m.compIdx >= len(matches) {
		m.compIdx = len(matches) - 1
	}
	if m.compIdx < 0 {
		m.compIdx = 0
	}
}

// acceptCompletion replaces the input with the highlighted command, leaving a
// trailing space so arguments can follow, and closes the menu.
func (m *chatModel) acceptCompletion() {
	if !m.compActive || len(m.compMatches) == 0 {
		return
	}
	c := m.compMatches[m.compIdx]
	m.ta.SetValue("/" + c.name + " ")
	m.ta.CursorEnd()
	m.compActive = false
	m.compMatches = nil
	m.compIdx = 0
}

// submit sends the current input: starting a turn when idle, or queueing it when
// the agent is busy.
func (m chatModel) submit() (tea.Model, tea.Cmd) {
	input := strings.TrimSpace(m.ta.Value())
	if input == "" {
		return m, nil
	}
	if input == "exit" || input == "quit" {
		return m, tea.Quit
	}
	m.ta.Reset()
	m.relayout()

	// Slash commands are handled by the UI and never recorded as user turns or
	// sent to the agent.
	if strings.HasPrefix(input, "/") {
		if model, cmd, handled := m.dispatchSlash(input); handled {
			return model, cmd
		}
	}

	m.repl.Session.Add(session.Entry{Kind: session.KindUser, Content: input})

	if m.running {
		m.queue = append(m.queue, input)
		m.append(m.styles.User.Render("› "+oneLine(input, 96)) + "  " + m.styles.Queued.Render("(queued)"))
		return m, nil
	}
	m.append(m.styles.User.Render("› " + input))
	return m.startTurn(input)
}

// startTurn launches the agent for one user message in a goroutine, forwarding
// events back as messages and reporting completion via turnDoneMsg.
func (m chatModel) startTurn(input string) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.phase = phaseThinking
	m.started = time.Now()

	p := m.repl.program
	history := m.history
	ev := m.repl.events(p)
	go func() {
		updated, err := m.repl.Agent.Run(ctx, history, input, ev)
		p.Send(turnDoneMsg{history: updated, err: err})
	}()
	return m, m.sp.Tick
}

// onTurnDone records the result and either starts the next queued message or
// returns to idle.
func (m chatModel) onTurnDone(msg turnDoneMsg) (tea.Model, tea.Cmd) {
	m.running = false
	m.cancel = nil
	m.history = msg.history
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			m.append("  " + m.styles.Denied.Render("⊘ interrupted"))
		} else {
			m.append("  " + m.styles.Error.Render("error: "+msg.err.Error()))
		}
		return m, nil // do not auto-compact on a failed turn
	}
	// Optimistically compact early when the prompt is approaching the window.
	// Queued messages drain after the compaction completes, against the smaller
	// history.
	if m.shouldAutoCompact() {
		m.warnedTightBudget = false
		return m.startCompaction("", true)
	}
	// Over the budget but shouldAutoCompact declined: the load is fixed overhead
	// (system prompt + connected tools), not summarizable conversation, so
	// compacting again would only reproduce a similar seed — the must-compact
	// loop. Warn once instead of looping silently; re-arm once we drop back under.
	if m.overBudget() {
		if !m.warnedTightBudget {
			m.warnedTightBudget = true
			m.append("  " + m.styles.Denied.Render("⚠ near the context window limit, but the load is mostly fixed overhead "+
				"(system prompt + connected tools) rather than conversation — compaction can't reclaim it. "+
				"Switch to a larger-window model with /connect or disable unused MCP tools."))
		}
	} else {
		m.warnedTightBudget = false
	}
	if len(m.queue) > 0 {
		next := m.queue[0]
		m.queue = m.queue[1:]
		m.append(m.styles.User.Render("› " + next))
		return m.startTurn(next)
	}
	return m, nil
}

// startCompaction launches a conversation summary in a goroutine, mirroring
// startTurn. It works for both manual /compact (auto=false) and optimistic
// auto-compaction (auto=true).
func (m chatModel) startCompaction(focus string, auto bool) (tea.Model, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running = true
	m.compacting = true
	m.phase = "Compacting conversation"
	m.started = time.Now()
	if auto {
		m.append("  " + m.styles.Hint.Render("● auto-compacting context (approaching window limit)…"))
	} else {
		m.append("  " + m.styles.Hint.Render("● compacting conversation…"))
	}

	p := m.repl.program
	history := m.history
	go func() {
		newHist, summary, err := m.repl.Agent.Compact(ctx, history, focus)
		p.Send(compactDoneMsg{history: newHist, summary: summary, auto: auto, err: err})
	}()
	return m, m.sp.Tick
}

// onCompactDone records the compaction result, swaps in the summarized history,
// and drains any messages queued while it ran.
func (m chatModel) onCompactDone(msg compactDoneMsg) (tea.Model, tea.Cmd) {
	m.running = false
	m.compacting = false
	m.cancel = nil
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			m.append("  " + m.styles.Denied.Render("⊘ compaction interrupted"))
		} else {
			m.append("  " + m.styles.Error.Render("compaction failed: "+msg.err.Error()))
		}
		return m, nil
	}
	priorMsgs := len(m.history)
	priorTokens := m.lastInputTokens
	m.history = msg.history
	m.compactedLen = len(msg.history) // baseline so we don't re-compact the bare seed
	m.lastInputTokens = 0             // next turn re-measures the smaller prompt
	m.warnedTightBudget = false
	// The recap is context for the model, not the operator: seed it into the new
	// history (already done via msg.history) and keep the full text in the session
	// log for the record, but show only a one-line confirmation in the transcript.
	m.repl.Session.Add(session.Entry{Kind: session.KindAssistant, Content: "[context compacted]\n\n" + msg.summary})
	m.append("  " + m.styles.Hint.Render(compactNotice(priorMsgs, priorTokens)))
	if len(m.queue) > 0 {
		next := m.queue[0]
		m.queue = m.queue[1:]
		m.append(m.styles.User.Render("› " + next))
		return m.startTurn(next)
	}
	return m, nil
}

// minCompactGain is how many messages must accumulate beyond the last
// compaction's seed before another auto-compaction is worthwhile. Re-summarizing
// a history that has barely grown only reproduces a similar-size seed — the
// must-compact loop the operator hits when fixed overhead dominates the window.
const minCompactGain = 2

// shouldAutoCompact reports whether the latest prompt has crossed the configured
// fraction of the context window AND enough conversation has accrued since the
// last compaction to make summarizing it worthwhile. Disabled when either
// threshold is non-positive or when there is no history to compact.
func (m chatModel) shouldAutoCompact() bool {
	if !m.overBudget() || len(m.history) == 0 {
		return false
	}
	// Nothing meaningful has accumulated since the last compaction (or session
	// start): compacting again cannot shrink the prompt, so don't.
	return len(m.history) > m.compactedLen+minCompactGain
}

// overBudget reports whether the latest prompt size has crossed the configured
// fraction of the context window, regardless of whether compaction can help.
func (m chatModel) overBudget() bool {
	win := m.repl.ContextWindow
	frac := m.repl.AutoCompactThreshold
	if win <= 0 || frac <= 0 {
		return false
	}
	return m.lastInputTokens >= int64(float64(win)*frac)
}

// compactNotice is the one-line transcript confirmation shown after compaction.
// The full recap stays hidden — it is fed to the model, not pasted for the
// operator to scroll past.
func compactNotice(msgs int, tokens int64) string {
	s := "● context compacted — prior conversation summarized"
	if msgs > 0 {
		s += fmt.Sprintf(" (%d messages", msgs)
		if tokens > 0 {
			s += fmt.Sprintf(", ~%s tokens", humanCount(tokens))
		}
		s += " folded into a recap)"
	}
	return s
}

// humanCount renders a token count compactly: 1500 -> "2k", 1_200_000 -> "1.2M".
func humanCount(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.0fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// interrupt cancels the in-flight turn and resolves any pending permission
// request so the agent goroutine cannot deadlock.
func (m *chatModel) interrupt() {
	if m.perm != nil {
		m.perm.reply <- false
		m.perm = nil
		m.ta.Focus()
	}
	if m.cancel != nil {
		m.cancel()
	}
}

// answerPerm replies to the pending permission request, optionally allowlisting
// the tool for the rest of the session.
func (m *chatModel) answerPerm(allow, always bool) {
	if m.perm == nil {
		return
	}
	if allow && always {
		m.repl.Gate.AllowTool(m.perm.name)
	}
	m.perm.reply <- allow
	if !allow {
		m.append("  " + m.styles.Denied.Render("✗ denied") + "  " + m.styles.ToolMeta.Render(m.perm.name))
	}
	m.perm = nil
	m.ta.Focus()
}

// --- rendering ---

func (m chatModel) View() string {
	if !m.ready {
		return "starting vala…"
	}
	box := m.styles.InputBox
	if m.running {
		box = m.styles.InputBoxBusy
	}
	parts := []string{m.vp.View()}
	if menu := m.completionView(); menu != "" {
		parts = append(parts, menu)
	}
	parts = append(parts,
		m.statusLine(),
		box.Width(m.width-2).Render(m.ta.View()),
		m.footer(),
	)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

// completionView renders the slash-command autocomplete menu shown just above the
// input box, with the highlighted row reversed out so the selection is obvious.
func (m chatModel) completionView() string {
	if !m.compActive {
		return ""
	}
	items := m.compMatches
	if len(items) > maxCompletionRows {
		items = items[:maxCompletionRows]
	}
	var b strings.Builder
	for i, c := range items {
		name := "/" + c.name
		if i == m.compIdx {
			b.WriteString("  " + m.styles.CompletionSel.Render(" "+name+" ") + "  " + m.styles.Hint.Render(c.desc))
		} else {
			b.WriteString("  " + m.styles.CompletionName.Render(name) + "  " + m.styles.ToolMeta.Render(c.desc))
		}
		if i < len(items)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// completionHeight is the on-screen row count of the autocomplete menu, used to
// reserve space when sizing the viewport. Zero when the menu is hidden.
func (m chatModel) completionHeight() int {
	if !m.compActive {
		return 0
	}
	n := len(m.compMatches)
	if n > maxCompletionRows {
		n = maxCompletionRows
	}
	return n
}

// statusLine sits just above the input: the activity spinner while running, the
// permission request when one is pending, or a blank spacer when idle.
func (m chatModel) statusLine() string {
	if m.perm != nil {
		line := "  " + m.styles.Permission.Render("permission needed") + "  " + m.styles.ToolCall.Render(m.perm.name)
		if s := oneLine(m.perm.summary, 72); s != "" {
			line += "  " + m.styles.ToolMeta.Render(s)
		}
		return line
	}
	if !m.running {
		return ""
	}
	line := "  " + m.sp.View() + " " + m.styles.SpinnerLabel.Render(m.phase)
	if e := time.Since(m.started).Round(time.Second); e >= time.Second {
		line += " " + m.styles.Hint.Render("· "+e.String())
	}
	line += "  " + m.styles.Hint.Render("esc to interrupt")
	if n := len(m.queue); n > 0 {
		line += "  " + m.styles.Queued.Render(fmt.Sprintf("· %d queued", n))
	}
	return line
}

func (m chatModel) footer() string {
	if m.perm != nil {
		return "  " + m.styles.PermissionKey.Render("[y]es  ·  [n]o  ·  [a]lways allow this tool")
	}
	// Only the non-obvious control survives: the permission mode and how to cycle
	// it. Send/newline/scroll/quit are universal terminal conventions — showing
	// them is noise.
	return "  " + m.styles.Mode.Render("⏵⏵ "+m.modeLabel()) + "  " + m.styles.Hint.Render("shift+tab to cycle")
}

// modeLabel renders the gate's current permission disposition for the footer.
func (m chatModel) modeLabel() string {
	switch string(m.repl.Gate.Mode) {
	case "allow":
		return "auto-allow"
	case "deny":
		return "deny-all"
	default:
		return "ask"
	}
}

// banner is the curated session header, rendered as the first transcript block.
func (m chatModel) banner() string {
	var b strings.Builder
	b.WriteString("  " + m.styles.BannerTag.Render("vala") + "  " + m.styles.Hint.Render(m.repl.Model))
	if line := m.evidenceLine(); line != "" {
		b.WriteString("\n  " + line)
	}
	b.WriteString("\n  " + m.styles.Rule.Render(strings.Repeat("─", 52)))
	return b.String()
}

// evidenceLine renders the connected-evidence summary for the banner, e.g.
// "evidence · scanner ✓ 4 tools · wiz ✗ command not found". When no sources are
// configured it nudges the operator toward connecting one rather than leaving a
// silent gap. Failures are shown inline so a non-connecting source is never
// swallowed behind the alt-screen.
func (m chatModel) evidenceLine() string {
	if len(m.repl.Evidence) == 0 {
		return m.styles.Hint.Render("evidence · none connected · ") +
			m.styles.ToolMeta.Render("run `vala setup` to add a source")
	}
	parts := make([]string, 0, len(m.repl.Evidence))
	for _, e := range m.repl.Evidence {
		if e.OK() {
			parts = append(parts, m.styles.ToolCall.Render(e.Name)+" "+
				m.styles.Prompt.Render("✓")+" "+m.styles.ToolMeta.Render(fmt.Sprintf("%d tools", e.Tools)))
			continue
		}
		parts = append(parts, m.styles.ToolMeta.Render(e.Name)+" "+
			m.styles.Error.Render("✗")+" "+m.styles.ResultErr.Render(oneLine(e.Err.Error(), 60)))
	}
	return m.styles.Hint.Render("evidence · ") + strings.Join(parts, m.styles.Hint.Render(" · "))
}

// renderResult collapses a tool result to a single dim line — a snippet of the
// output, the way Claude Code previews an MCP response — with a "(+N lines)"
// marker when there is more. The full content is preserved in the session
// transcript on disk, so nothing is lost; the live view just stays uncluttered.
// Errors keep their first line in the error tone so failures are never silently
// swallowed.
func (m chatModel) renderResult(content string, isErr bool) string {
	trimmed := strings.TrimRight(content, "\n")
	if isErr {
		return m.styles.Error.Render("  ⎿ ") + m.styles.ResultErr.Render(oneLine(trimmed, 80))
	}
	connector := m.styles.ToolMeta.Render("  ⎿ ")
	if trimmed == "" {
		return connector + m.styles.Hint.Render("done")
	}
	first := trimmed
	if i := strings.IndexByte(trimmed, '\n'); i >= 0 {
		first = trimmed[:i]
	}
	out := connector + m.styles.Hint.Render(oneLine(first, 64))
	if extra := strings.Count(trimmed, "\n"); extra > 0 {
		out += m.styles.ToolMeta.Render(fmt.Sprintf("  (+%d lines)", extra))
	}
	return out
}

// append adds a rendered block to the transcript and scrolls to it.
func (m *chatModel) append(block string) {
	m.blocks = append(m.blocks, block)
	m.refreshViewport()
}

func (m *chatModel) refreshViewport() {
	if !m.ready {
		return
	}
	m.vp.SetContent(strings.Join(m.blocks, "\n"))
	m.vp.GotoBottom()
}

// resize recomputes the layout for a new terminal size.
func (m chatModel) resize(w, h int) (tea.Model, tea.Cmd) {
	m.width, m.height = w, h
	m.md = newMarkdownRenderer(w - 4)
	m.ta.SetWidth(w - 4)
	if !m.ready {
		m.vp = viewport.New(w, m.viewportHeight())
		m.ready = true
	}
	m.relayout()
	return m, nil
}

// relayout adjusts the input height to its content and resizes the viewport to
// fill the remaining space.
func (m *chatModel) relayout() {
	if !m.ready {
		return
	}
	m.ta.SetHeight(m.inputLines())
	m.vp.Width = m.width
	m.vp.Height = m.viewportHeight()
	m.refreshViewport()
}

// inputLines is the on-screen height of the input box: the number of rows the
// current value wraps to, clamped to maxInputLines. It counts soft-wrapped rows
// (not just hard newlines) so long text expands the box like Claude Code.
func (m chatModel) inputLines() int {
	lines := inputRows(m.ta.Value(), m.ta.Width())
	if lines > maxInputLines {
		lines = maxInputLines
	}
	return lines
}

// viewportHeight returns the rows available for the transcript after reserving
// space for the status line, bordered input box, and footer.
func (m chatModel) viewportHeight() int {
	lines := 1
	if m.ready {
		lines = m.inputLines()
	}
	inputBox := lines + 2 // rounded border top and bottom
	h := m.height - inputBox - 1 /*status*/ - 1 /*footer*/ - m.completionHeight()
	if h < 3 {
		h = 3
	}
	return h
}
