package ui

import (
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/agent"
	"github.com/brittonhayes/vala/internal/mcp"
	"github.com/brittonhayes/vala/internal/mode"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/session"
	"github.com/brittonhayes/vala/internal/tool"
	"github.com/brittonhayes/vala/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// newTestModel builds a chatModel wired to a real session and gate but no agent,
// then sizes it as if the terminal reported 80x24. Tests that avoid submit-while-
// idle never touch the (nil) agent.
func newTestModel(t *testing.T) chatModel {
	t.Helper()
	sess, err := session.New(t.TempDir())
	if err != nil {
		t.Fatalf("session.New: %v", err)
	}
	r := &REPL{
		Gate:    permission.New(permission.ModeAsk, nil),
		Session: sess,
		Model:   "test-model",
		styles:  DefaultStyles(),
	}
	m := newChatModel(r)
	mm, _ := m.resize(80, 24)
	return mm.(chatModel)
}

// TestQueueWhileRunning verifies that submitting a message during an active turn
// enqueues it instead of starting a second turn (which would hit the nil agent).
func TestQueueWhileRunning(t *testing.T) {
	m := newTestModel(t)
	m.running = true
	m.ta.SetValue("look at the auth logs")

	res, _ := m.submit()
	m = res.(chatModel)

	if len(m.queue) != 1 || m.queue[0] != "look at the auth logs" {
		t.Fatalf("expected message queued, got queue=%v", m.queue)
	}
	if m.ta.Value() != "" {
		t.Fatalf("expected input cleared after submit, got %q", m.ta.Value())
	}
}

// TestPermissionApprove checks that answering a pending permission request
// unblocks the waiting agent goroutine with the right verdict.
func TestPermissionApprove(t *testing.T) {
	m := newTestModel(t)
	reply := make(chan bool, 1)
	m.perm = &permMsg{name: "bash", summary: "ls", reply: reply}

	m.answerPerm(true)

	select {
	case got := <-reply:
		if !got {
			t.Fatal("expected approve=true")
		}
	default:
		t.Fatal("expected a reply on the channel")
	}
	if m.perm != nil {
		t.Fatal("expected perm cleared after answer")
	}
}

func TestPermissionLineIsCompact(t *testing.T) {
	m := newTestModel(t)
	reply := make(chan bool, 1)
	m.perm = &permMsg{name: "edit", summary: "detections/x.yml", reply: reply}

	line := m.permissionLine()
	for _, want := range []string{"approve", "edit", "detections/x.yml", "Y", "N"} {
		if !strings.Contains(line, want) {
			t.Fatalf("permission line missing %q: %q", want, line)
		}
	}
	if strings.Contains(line, "permission needed") || strings.Contains(line, "always") {
		t.Fatalf("permission line is too noisy: %q", line)
	}
	if footer := m.footer(); footer != "" {
		t.Fatalf("permission footer should be hidden while approval is pending: %q", footer)
	}
}

func TestPermissionLineWrapsDecisionText(t *testing.T) {
	m := newTestModel(t)
	res, _ := m.resize(64, 24)
	m = res.(chatModel)
	reply := make(chan bool, 1)
	m.perm = &permMsg{
		name:    "validate_data",
		summary: "completeness: CloudTrail management events ingested and queryable for App Prod api with enough retention to decide whether IMDS credentials were reused",
		reply:   reply,
	}

	line := m.permissionLine()
	for _, want := range []string{"approve", "validate_data", "CloudTrail", "IMDS credentials", "Y", "N"} {
		if !strings.Contains(line, want) {
			t.Fatalf("permission line missing %q:\n%s", want, line)
		}
	}
	for _, row := range strings.Split(line, "\n") {
		if got := lipgloss.Width(row); got > m.width {
			t.Fatalf("permission row width = %d, want <= %d:\n%s\n\nfull prompt:\n%s", got, m.width, row, line)
		}
	}
	if h := m.statusHeight(); h < 3 {
		t.Fatalf("statusHeight = %d, expected wrapped approval prompt to reserve multiple rows:\n%s", h, line)
	}
}

func TestChoiceMultiSelectSubmitsDefaultsAndToggles(t *testing.T) {
	m := newTestModel(t)
	reply := make(chan tools.ChoiceResponse, 1)
	m.choice = newChoicePrompt(choiceMsg{
		req: tools.ChoiceRequest{
			Question: "Proceed?",
			Mode:     tools.ChoiceMulti,
			Options: []tools.ChoiceOption{
				{ID: "A", Label: "Store hunt", Default: true},
				{ID: "B", Label: "Update coverage"},
				{ID: "C", Label: "Remember", Default: true},
			},
			AllowChat: true,
		},
		reply: reply,
	})

	res, _ := m.onChoiceKey(tea.KeyMsg{Type: tea.KeyDown})
	m = res.(chatModel)
	res, _ = m.onChoiceKey(tea.KeyMsg{Type: tea.KeySpace})
	m = res.(chatModel)
	res, _ = m.onChoiceKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(chatModel)

	got := <-reply
	if strings.Join(got.Selected, ",") != "A,B,C" {
		t.Fatalf("selected = %v, want A,B,C", got.Selected)
	}
	if m.choice != nil {
		t.Fatal("choice should be cleared after answer")
	}
}

func TestChoiceChatFallback(t *testing.T) {
	m := newTestModel(t)
	reply := make(chan tools.ChoiceResponse, 1)
	m.choice = newChoicePrompt(choiceMsg{
		req: tools.ChoiceRequest{
			Question:  "Proceed?",
			Mode:      tools.ChoiceSingle,
			Options:   []tools.ChoiceOption{{ID: "A", Label: "Proceed"}},
			AllowChat: true,
		},
		reply: reply,
	})

	res, _ := m.onChoiceKey(tea.KeyMsg{Type: tea.KeyTab})
	m = res.(chatModel)
	if !m.choice.chatting {
		t.Fatal("expected choice to enter chat mode")
	}
	m.ta.SetValue("Only do the memory write")
	res, _ = m.onChoiceKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = res.(chatModel)

	got := <-reply
	if got.Message != "Only do the memory write" || len(got.Selected) != 0 {
		t.Fatalf("unexpected chat response: %#v", got)
	}
}

func TestChoiceViewWrapsLongRows(t *testing.T) {
	m := newTestModel(t)
	res, _ := m.resize(64, 24)
	m = res.(chatModel)
	m.choice = newChoicePrompt(choiceMsg{
		req: tools.ChoiceRequest{
			Question: "I've scoped the next hunt to confirmed App Prod api cryptojacking and IMDS credential theft incident. Pick the next write.",
			Mode:     tools.ChoiceMulti,
			Options: []tools.ChoiceOption{
				{
					ID:      "creds_outside_aws",
					Label:   "Blast radius: were the scraped api ECS task-role creds used outside AWS?",
					Detail:  "This needs to remain readable in a narrow terminal instead of running past the right edge.",
					Default: true,
				},
				{
					ID:     "assume_role_enum",
					Label:  "Privilege escalation: per-actor AssumeRole enumeration versus baseline.",
					Detail: "Another long detail that should wrap under the row body.",
				},
			},
			AllowChat: true,
		},
		reply: make(chan tools.ChoiceResponse, 1),
	})

	view := m.choiceView()
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > m.width {
			t.Fatalf("choice line width = %d, want <= %d:\n%s\n\nfull view:\n%s", got, m.width, line, view)
		}
	}
	if h := m.choiceHeight(); h <= 1+len(m.choice.req.Options)+1 {
		t.Fatalf("choiceHeight = %d, expected wrapped rows to reserve extra height\n%s", h, view)
	}
}

// TestInterruptResolvesPendingPermission ensures cancelling a turn also releases
// a goroutine blocked on a permission reply, so it cannot deadlock.
func TestInterruptResolvesPendingPermission(t *testing.T) {
	m := newTestModel(t)
	reply := make(chan bool, 1)
	m.perm = &permMsg{name: "bash", summary: "rm -rf /", reply: reply}
	canceled := false
	m.cancel = func() { canceled = true }

	m.interrupt()

	if got := <-reply; got {
		t.Fatal("expected interrupt to deny the pending permission")
	}
	if !canceled {
		t.Fatal("expected interrupt to cancel the turn context")
	}
	if m.perm != nil {
		t.Fatal("expected perm cleared after interrupt")
	}
}

func TestRenderResultUsesRichCard(t *testing.T) {
	m := newTestModel(t)

	out := m.renderResult(tool.Result{Content: "model-facing text", Card: &tool.Card{
		Title:   "Finding recorded",
		Summary: "A cited evidence row was added.",
		Fields: []tool.Field{
			{Label: "claim", Value: "Okta impossible travel candidate observed"},
			{Label: "evidence ID", Value: "evidence_123"},
		},
		Changes: []tool.Change{{Label: "finding", After: "Okta impossible travel candidate observed"}},
		Suggestions: []tool.Suggestion{{
			Title:      "Close the visibility gap",
			Trigger:    "Visibility gap",
			Hypothesis: "Telemetry can be restored.",
			DataSource: "cloudtrail",
			Priority:   "high",
		}},
	}})

	for _, want := range []string{"Finding recorded", "finding added", "claim", "evidence_123", "queue next"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rich card missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "model-facing text") {
		t.Fatalf("rich card should not render raw model content:\n%s", out)
	}
}

func TestRenderResultFallbackAndError(t *testing.T) {
	m := newTestModel(t)

	fallback := m.renderResult(tool.Text("first line\nsecond line"))
	if !strings.Contains(fallback, "first line") || !strings.Contains(fallback, "+1 lines") {
		t.Fatalf("fallback missing compact output:\n%s", fallback)
	}

	errOut := m.renderResult(tool.Result{
		Content: "failed loudly",
		IsError: true,
		Card:    &tool.Card{Title: "Should not render"},
	})
	if !strings.Contains(errOut, "failed loudly") || strings.Contains(errOut, "Should not render") {
		t.Fatalf("error should keep compact error styling and ignore cards:\n%s", errOut)
	}
}

func TestWideViewStaysFullWidthWithoutRail(t *testing.T) {
	m := newTestModel(t)
	res, _ := m.resize(120, 24)
	m = res.(chatModel)
	view := m.View()

	if strings.Contains(view, "Active Hunt") || strings.Contains(view, "detection workspace") {
		t.Fatalf("view rendered removed rail:\n%s", view)
	}
	if m.vp.Width != 120 {
		t.Fatalf("wide viewport width = %d, want full terminal width 120", m.vp.Width)
	}
}

func TestComposerIsFlatFullWidthAndGuttered(t *testing.T) {
	m := newTestModel(t)
	m.ta.SetValue("hunt cloudtrail")

	view := ansi.Strip(m.View())
	for _, glyph := range []string{"╭", "╮", "╰", "╯", "─", "│"} {
		if strings.Contains(view, glyph) {
			t.Fatalf("composer should not render box borders %q:\n%s", glyph, view)
		}
	}
	if !strings.Contains(view, "\n"+uiGutter+"› hunt cloudtrail") {
		t.Fatalf("composer prompt should sit on the shared gutter:\n%s", view)
	}
	if got, want := lipgloss.Width(uiGutter)+lipgloss.Width(m.ta.Prompt)+m.ta.Width(), m.width; got != want {
		t.Fatalf("visual composer width = %d, want %d", got, want)
	}
	rows := strings.Split(view, "\n")
	composerRow := -1
	for i, row := range rows {
		if strings.Contains(row, uiGutter+"› hunt cloudtrail") {
			composerRow = i
			break
		}
	}
	if composerRow < 0 || composerRow+1 >= len(rows) || strings.TrimSpace(rows[composerRow+1]) != "" {
		t.Fatalf("view should reserve padding between composer and footer:\n%s", view)
	}
	if strings.TrimSpace(rows[len(rows)-1]) != "" {
		t.Fatalf("view should reserve bottom padding:\n%s", view)
	}
}

func TestChromeShowsBrandModelDefaultPermissionsAndMode(t *testing.T) {
	m := newTestModel(t)
	m.repl.Agent = agent.New(nil, tool.NewRegistry(), permission.New(permission.ModeAsk, nil), "", 1, 1, "", agent.Session{Mode: mode.Default()})
	m.repl.Model = "anthropic · claude-sonnet-4"
	m.repl.Evidence = []mcp.EvidenceStatus{{Name: "wiz", Tools: 192}}
	m.refreshViewport()

	banner := ansi.Strip(m.banner())
	if strings.Contains(banner, "permissions") || strings.Contains(banner, "mode:") {
		t.Fatalf("banner should only show brand/model chrome:\n%s", banner)
	}

	view := ansi.Strip(m.View())
	for _, want := range []string{"◇ vala", "claude-sonnet-4", "shift+tab to cycle permissions", "mode: hunt"} {
		if !strings.Contains(view, want) {
			t.Fatalf("chrome missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"anthropic", "evidence", "wiz", "192 tools", "ask mode", "auto mode", "permissions: auto", "auto-accept permissions", "▸▸", "← for agents"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("chrome should not contain %q:\n%s", unwanted, view)
		}
	}
}

func TestChromeShowsAutoAcceptPermissionsOnlyInAuto(t *testing.T) {
	m := newTestModel(t)
	m.repl.Gate.Mode = permission.ModeAuto
	m.refreshViewport()

	view := ansi.Strip(m.View())
	for _, want := range []string{"permissions: auto", "(shift+tab to cycle permissions)"} {
		if !strings.Contains(view, want) {
			t.Fatalf("auto chrome missing %q:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"ask mode", "auto mode", "auto-accept permissions", " permissions on", "▸▸", "pauses for permissions", "← for agents"} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("auto chrome should not contain %q:\n%s", unwanted, view)
		}
	}
}

func TestShiftTabRefreshesPermissionFooter(t *testing.T) {
	m := newTestModel(t)

	before := ansi.Strip(m.View())
	if !strings.Contains(before, "shift+tab to cycle permissions") {
		t.Fatalf("starting chrome should show permissions shortcut:\n%s", before)
	}
	if strings.Contains(before, "permissions: auto") {
		t.Fatalf("default permissions should not show auto-accept state:\n%s", before)
	}

	res, _ := m.onKey(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = res.(chatModel)

	after := ansi.Strip(m.View())
	if !strings.Contains(after, "permissions: auto") {
		t.Fatalf("shift+tab should refresh footer to auto permissions:\n%s", after)
	}
	if strings.Contains(after, "ask mode") || strings.Contains(after, "auto mode") {
		t.Fatalf("permissions footer should not use mode wording:\n%s", after)
	}
}
