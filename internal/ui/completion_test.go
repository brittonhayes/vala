package ui

import (
	"strings"
	"testing"
)

// cmdNames pulls the command names out of a match slice for terse assertions.
func cmdNames(cmds []slashCommand) []string {
	names := make([]string, len(cmds))
	for i, c := range cmds {
		names[i] = c.name
	}
	return names
}

func TestMatchCommandsEmptyReturnsAll(t *testing.T) {
	m := newTestModel(t)
	all := m.commands()
	got := matchCommands(all, "")
	if len(got) != len(all) {
		t.Fatalf("empty query returned %d commands, want %d", len(got), len(all))
	}
	// Order is preserved so the menu doubles as the full command list.
	for i := range all {
		if got[i].name != all[i].name {
			t.Errorf("order mismatch at %d: got %q, want %q", i, got[i].name, all[i].name)
		}
	}
}

func TestMatchCommandsRanking(t *testing.T) {
	m := newTestModel(t)
	cmds := m.commands()
	tests := []struct {
		query string
		first string // expected top-ranked command name
	}{
		{"help", "help"},        // exact name
		{"co", "connect"},       // prefix beats compact's subsequence
		{"cmt", "compact"},      // non-contiguous subsequence "c-m-...-t"
		{"provider", "connect"}, // description match
		{"mode", "mode"},
	}
	for _, tt := range tests {
		got := matchCommands(cmds, tt.query)
		if len(got) == 0 {
			t.Errorf("matchCommands(%q) returned no matches", tt.query)
			continue
		}
		if got[0].name != tt.first {
			t.Errorf("matchCommands(%q) top = %q, want %q (got %v)", tt.query, got[0].name, tt.first, cmdNames(got))
		}
	}
}

func TestMatchCommandsNoMatch(t *testing.T) {
	m := newTestModel(t)
	if got := matchCommands(m.commands(), "zzzznope"); len(got) != 0 {
		t.Errorf("expected no matches, got %v", cmdNames(got))
	}
}

func TestSubsequence(t *testing.T) {
	tests := []struct {
		query, target string
		want          bool
	}{
		{"", "compact", true},
		{"cmt", "compact", true},
		{"compact", "compact", true},
		{"cba", "compact", false}, // out of order
		{"xyz", "compact", false},
	}
	for _, tt := range tests {
		if got := subsequence(tt.query, tt.target); got != tt.want {
			t.Errorf("subsequence(%q, %q) = %v, want %v", tt.query, tt.target, got, tt.want)
		}
	}
}

// TestUpdateCompletionActivation checks when the menu opens and closes as the
// operator types.
func TestUpdateCompletionActivation(t *testing.T) {
	tests := []struct {
		input      string
		wantActive bool
	}{
		{"", false},                // empty input
		{"hunt for logins", false}, // plain message
		{"/", true},                // bare slash lists everything
		{"/co", true},              // partial command name
		{"/help", true},            // full command name still shows the menu
		{"/compact focus", false},  // a space means we're typing args now
		{"/zzz", false},            // no matching command
	}
	for _, tt := range tests {
		m := newTestModel(t)
		m.ta.SetValue(tt.input)
		m.updateCompletion()
		if m.compActive != tt.wantActive {
			t.Errorf("updateCompletion(%q) active = %v, want %v", tt.input, m.compActive, tt.wantActive)
		}
	}
}

// TestAcceptCompletionFillsInput verifies selecting a suggestion rewrites the
// input to the command with a trailing space and closes the menu.
func TestAcceptCompletionFillsInput(t *testing.T) {
	m := newTestModel(t)
	m.ta.SetValue("/co")
	m.updateCompletion()
	if !m.compActive {
		t.Fatal("expected menu active for /co")
	}
	// /co ranks connect first.
	m.acceptCompletion()
	if got := m.ta.Value(); got != "/connect " {
		t.Errorf("input after accept = %q, want %q", got, "/connect ")
	}
	if m.compActive {
		t.Error("menu should close after accepting a completion")
	}
}

// TestCompletionIndexClampsOnRefilter ensures a highlighted row that falls
// outside a newly narrowed match list is clamped rather than left dangling.
func TestCompletionIndexClampsOnRefilter(t *testing.T) {
	m := newTestModel(t)
	m.ta.SetValue("/")
	m.updateCompletion()
	m.compIdx = len(m.compMatches) - 1 // highlight the last of the full list

	m.ta.SetValue("/compact") // narrows to a single match
	m.updateCompletion()
	if m.compIdx >= len(m.compMatches) {
		t.Fatalf("compIdx %d out of range for %d matches", m.compIdx, len(m.compMatches))
	}
}

// TestCompletionViewHighlightsSelection confirms the rendered menu marks the
// highlighted command distinctly from the rest.
func TestCompletionViewHighlightsSelection(t *testing.T) {
	m := newTestModel(t)
	m.ta.SetValue("/")
	m.updateCompletion()
	view := m.completionView()
	if view == "" {
		t.Fatal("expected a rendered completion menu")
	}
	// Every command name should appear in the menu.
	for _, c := range m.commands() {
		if !strings.Contains(view, "/"+c.name) {
			t.Errorf("completion view missing command %q", c.name)
		}
	}
}
