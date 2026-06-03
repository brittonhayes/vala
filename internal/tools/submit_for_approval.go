package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed submit_for_approval.md
var submitForApprovalDescription string

// SubmitForApproval signals that the model is done proposing actions and the run
// should advance to the approval gate. Class: control (Propose phase). It sets a
// flag the engine reads to transition phases; it never executes anything.
type SubmitForApproval struct{ RC *RunContext }

func (t *SubmitForApproval) Name() string        { return "submit_for_approval" }
func (t *SubmitForApproval) Description() string { return submitForApprovalDescription }
func (t *SubmitForApproval) ReadOnly() bool      { return false }

func (t *SubmitForApproval) Schema() tool.Schema {
	return tool.Schema{Properties: map[string]any{}, Required: nil}
}

func (t *SubmitForApproval) Run(_ context.Context, _ json.RawMessage) (tool.Result, error) {
	t.RC.markSubmitted()
	n := len(t.RC.Actions())
	return tool.Text(fmt.Sprintf("submitted %d proposed action(s) for approval", n)), nil
}
