package mode

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/brittonhayes/vala/internal/tool"
)

// stubTool is a minimal tool.Tool for exercising mode policies by name.
type stubTool struct{ name string }

func (s stubTool) Name() string        { return s.name }
func (s stubTool) Description() string { return "" }
func (s stubTool) Schema() tool.Schema { return tool.Schema{} }
func (s stubTool) ReadOnly() bool      { return true }
func (s stubTool) Run(context.Context, json.RawMessage) (tool.Result, error) {
	return tool.Result{}, nil
}

func TestGetAndDefault(t *testing.T) {
	if _, ok := Get("hunt"); !ok {
		t.Error("hunt mode should be registered")
	}
	if _, ok := Get("detect"); !ok {
		t.Error("detect mode should be registered")
	}
	if _, ok := Get("nope"); ok {
		t.Error("unknown mode should not resolve")
	}
	if Default().ID != DefaultID || DefaultID != "hunt" {
		t.Errorf("default mode = %q, want hunt", Default().ID)
	}
}

func TestAllOrderingHuntFirst(t *testing.T) {
	all := All()
	if len(all) < 2 {
		t.Fatalf("expected at least hunt and detect, got %d modes", len(all))
	}
	if all[0].ID != "hunt" {
		t.Errorf("first mode = %q, want hunt", all[0].ID)
	}
	if all[1].ID != "detect" {
		t.Errorf("second mode = %q, want detect", all[1].ID)
	}
}

func TestDetectPolicyDropsHuntLifecycle(t *testing.T) {
	m, _ := Get("detect")
	dropped := []string{"open_hunt", "validate_data", "store_hunt", "update_coverage", "queue_hunt", "record_finding", "record_intel", "link_artifacts"}
	for _, name := range dropped {
		if m.ToolPolicy(stubTool{name}) {
			t.Errorf("detect should drop %q", name)
		}
	}
	kept := []string{"validate_detection", "edit_detection_logic", "read", "bash", "recall"}
	for _, name := range kept {
		if !m.ToolPolicy(stubTool{name}) {
			t.Errorf("detect should keep %q", name)
		}
	}
}

func TestHuntPolicyIsNil(t *testing.T) {
	m, _ := Get("hunt")
	if m.ToolPolicy != nil {
		t.Error("hunt mode should expose all tools (nil policy)")
	}
}
