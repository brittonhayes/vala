package runner

import (
	"context"
	"encoding/json"

	"github.com/brittonhayes/vala/internal/tool"
)

// mockAction is a generic action-class tool used only by the harness so that
// scenarios can exercise approval-required actions (e.g. github_issue) that have
// no real integration in v1. It always succeeds and records nothing.
type mockAction struct{ name string }

func (m mockAction) Name() string        { return m.name }
func (m mockAction) Description() string { return "harness mock action: " + m.name }
func (m mockAction) ReadOnly() bool      { return false }
func (m mockAction) Schema() tool.Schema {
	return tool.Schema{Properties: map[string]any{}, Required: nil}
}
func (m mockAction) Run(_ context.Context, _ json.RawMessage) (tool.Result, error) {
	return tool.Text("mock action " + m.name + " executed"), nil
}
