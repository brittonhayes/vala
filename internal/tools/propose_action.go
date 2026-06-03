package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/governance"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed propose_action.md
var proposeActionDescription string

// ProposeAction records an explicit proposal to run a write/destructive action.
// It does NOT execute anything: it computes a stable action ID, validates the
// policy evidence requirement, and records the proposal in the ledger and the
// Actions database with status "proposed". Class: control (Propose phase only).
type ProposeAction struct{ RC *RunContext }

func (t *ProposeAction) Name() string        { return "propose_action" }
func (t *ProposeAction) Description() string { return proposeActionDescription }
func (t *ProposeAction) ReadOnly() bool      { return false }

func (t *ProposeAction) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"tool":         map[string]any{"type": "string", "description": "The action tool to run if approved, e.g. \"slack_notify\"."},
			"input":        map[string]any{"type": "object", "description": "The exact JSON input the action tool will run with."},
			"rationale":    map[string]any{"type": "string", "description": "Why this action is warranted."},
			"evidence_ids": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Evidence IDs that justify the action."},
		},
		Required: []string{"tool", "input", "rationale"},
	}
}

func (t *ProposeAction) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Tool        string          `json:"tool"`
		Input       json.RawMessage `json:"input"`
		Rationale   string          `json:"rationale"`
		EvidenceIDs []string        `json:"evidence_ids"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Tool == "" || in.Rationale == "" {
		return tool.Errorf("tool and rationale are required"), nil
	}
	if len(in.Input) == 0 {
		in.Input = json.RawMessage("{}")
	}

	// Enforce the policy evidence requirement and that cited evidence exists.
	if t.RC.Policy.RequiresEvidence(in.Tool) {
		if len(in.EvidenceIDs) == 0 {
			return tool.Errorf("action %q requires at least one evidence_id; record evidence first", in.Tool), nil
		}
		for _, id := range in.EvidenceIDs {
			if !t.RC.knownEvidence(id) {
				return tool.Errorf("unknown evidence id %q; record it before citing it", id), nil
			}
		}
	}

	id := governance.ActionID(in.Tool, in.Input)
	class := string(t.RC.Policy.ClassOf(in.Tool))
	t.RC.Ledger.Propose(governance.ProposedAction{
		ID: id, Tool: in.Tool, Input: in.Input, Class: class,
		Rationale: in.Rationale, Evidence: in.EvidenceIDs,
	})
	a := &brain.Action{ID: id, Class: in.Tool, Params: string(in.Input), Rationale: in.Rationale, Status: governance.StatusProposed, Evidence: in.EvidenceIDs}
	rowID, err := t.RC.Brain.RecordAction(ctx, t.RC.CaseID, *a)
	if err != nil {
		return tool.Errorf("failed to record action: %v", err), nil
	}
	t.RC.addAction(a, rowID)
	return tool.Text(fmt.Sprintf("proposed action %s (%s) — pending approval", id, in.Tool)), nil
}
