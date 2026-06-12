package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/mode"
	"github.com/brittonhayes/vala/internal/permission"
	"github.com/brittonhayes/vala/internal/skills"
	"github.com/brittonhayes/vala/internal/tool"
)

// allowAll is a decideFunc that approves every call, isolating the mode-exposure
// check from the permission gate.
func allowAll(llm.Block, string, tool.Tool) (bool, string) { return true, "" }

// fakeTool is a minimal named tool for building a registry in tests.
type fakeTool struct {
	name     string
	readOnly bool
	ran      *bool
}

func (f fakeTool) Name() string        { return f.name }
func (f fakeTool) Description() string { return f.name }
func (f fakeTool) Schema() tool.Schema { return tool.Schema{} }
func (f fakeTool) ReadOnly() bool      { return f.readOnly }
func (f fakeTool) Run(context.Context, json.RawMessage) (tool.Result, error) {
	if f.ran != nil {
		*f.ran = true
	}
	return tool.Text("ran " + f.name), nil
}

// testRegistry builds a registry covering hunt-lifecycle, detection, evidence,
// and skill tools so a mode filter can be exercised end to end.
func testRegistry() *tool.Registry {
	r := tool.NewRegistry()
	r.Register(
		fakeTool{name: "open_hunt"},
		fakeTool{name: "store_hunt"},
		fakeTool{name: "validate_detection", readOnly: true},
		fakeTool{name: "edit_detection_logic"},
		fakeTool{name: "scanner_execute_query", readOnly: true}, // MCP evidence
		fakeTool{name: "skill", readOnly: true},
	)
	return r
}

func newAgent(t *testing.T, m mode.Mode) *Agent {
	t.Helper()
	sk := skills.NewSet(skills.Skill{Name: "sigma-authoring", Description: "checklist"})
	return New(nil, testRegistry(), permission.New(permission.ModeAllow, nil), "/w", 10, 1,
		"", Session{Mode: m, Skills: sk, EvidenceNames: []string{"scanner_execute_query"}})
}

func exposed(a *Agent) map[string]bool {
	out := map[string]bool{}
	for _, name := range a.exposedToolNames() {
		out[name] = true
	}
	return out
}

func TestDetectModeFiltersTools(t *testing.T) {
	detect, _ := mode.Get("detect")
	a := newAgent(t, detect)
	got := exposed(a)

	for _, name := range []string{"open_hunt", "store_hunt"} {
		if got[name] {
			t.Errorf("detect mode should hide %q", name)
		}
	}
	for _, name := range []string{"validate_detection", "edit_detection_logic", "skill"} {
		if !got[name] {
			t.Errorf("detect mode should expose %q", name)
		}
	}
	// Evidence is always exposed regardless of the mode policy.
	if !got["scanner_execute_query"] {
		t.Error("MCP evidence tool must stay exposed in detect mode")
	}
}

func TestHuntModeExposesAllButSkill(t *testing.T) {
	hunt := mode.Default()
	a := newAgent(t, hunt)
	got := exposed(a)

	for _, name := range []string{"open_hunt", "store_hunt", "validate_detection", "edit_detection_logic", "scanner_execute_query"} {
		if !got[name] {
			t.Errorf("hunt mode should expose %q", name)
		}
	}
	// hunt bundles no skills, so the skill tool is hidden even though it is
	// registered — keeping hunt's exposed tool set unchanged from before modes.
	if got["skill"] {
		t.Error("hunt mode should hide the skill tool (no bundled skills)")
	}
}

func TestSetModeSwapsExposureLive(t *testing.T) {
	a := newAgent(t, mode.Default())
	if !exposed(a)["open_hunt"] {
		t.Fatal("hunt should expose open_hunt")
	}
	detect, _ := mode.Get("detect")
	a.SetMode(detect)
	if exposed(a)["open_hunt"] {
		t.Error("after switching to detect, open_hunt should be hidden")
	}
	if !exposed(a)["skill"] {
		t.Error("after switching to detect, skill should be exposed")
	}
	if a.Mode().ID != "detect" {
		t.Errorf("Mode() = %q, want detect", a.Mode().ID)
	}
}

func TestRunToolUseRefusesFilteredTool(t *testing.T) {
	ran := false
	r := tool.NewRegistry()
	r.Register(fakeTool{name: "open_hunt", ran: &ran})
	detect, _ := mode.Get("detect")
	a := New(nil, r, permission.New(permission.ModeAllow, nil), "/w", 10, 1, "",
		Session{Mode: detect, Skills: skills.NewSet(), EvidenceNames: nil})

	// open_hunt is hidden in detect mode; naming it directly must not execute.
	block := llm.ToolUseBlock("id-1", "open_hunt", json.RawMessage(`{}`))
	res := a.runToolUse(context.Background(), block, allowAll, Events{})
	if !res.IsError {
		t.Error("calling a filtered-out tool should return an error result")
	}
	if !strings.Contains(res.Content, "not available in detect mode") {
		t.Errorf("unexpected refusal message: %q", res.Content)
	}
	if ran {
		t.Error("a filtered-out tool must not run")
	}
}
