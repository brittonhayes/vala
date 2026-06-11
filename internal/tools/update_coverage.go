package tools

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed update_coverage.md
var updateCoverageDescription string

// UpdateCoverage is the feedback stage of the loop: it records or updates the
// detection-coverage state for an ATT&CK technique so the next hunt can be aimed
// at the weakest spots. It upserts a Coverage row keyed by technique and links
// it to the active hunt.
type UpdateCoverage struct{ RC *RunContext }

func (t *UpdateCoverage) Name() string        { return "update_coverage" }
func (t *UpdateCoverage) Description() string { return updateCoverageDescription }
func (t *UpdateCoverage) ReadOnly() bool      { return false }

func (t *UpdateCoverage) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"technique":  map[string]any{"type": "string", "description": "ATT&CK technique ID, e.g. attack.t1562.001."},
			"tactic":     map[string]any{"type": "string", "description": "The ATT&CK tactic, e.g. defense-evasion (optional)."},
			"status":     map[string]any{"type": "string", "enum": []string{brain.CoverageCovered, brain.CoverageThin, brain.CoverageUncovered}, "description": "Coverage state after this hunt: Covered, Thin, or Uncovered."},
			"fidelity":   map[string]any{"type": "string", "enum": []string{"high", "medium", "low", "none"}, "description": "Detection fidelity for this technique. Tie to the hunt's detection tier: tier1->high, tier2->medium, tier3->low, tier4/5->none."},
			"detections": map[string]any{"type": "string", "description": "Summary of the detections that cover this technique (optional)."},
		},
		Required: []string{"technique", "status"},
	}
}

func (t *UpdateCoverage) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Technique  string `json:"technique"`
		Tactic     string `json:"tactic"`
		Status     string `json:"status"`
		Fidelity   string `json:"fidelity"`
		Detections string `json:"detections"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Technique == "" {
		return tool.Errorf("technique is required (an ATT&CK ID, e.g. attack.t1562.001)"), nil
	}
	if t.RC == nil || t.RC.Brain == nil {
		return tool.Errorf("no brain configured to update coverage in"), nil
	}
	id, err := t.RC.Brain.UpsertCoverage(ctx, brain.Coverage{
		Technique:  in.Technique,
		Tactic:     in.Tactic,
		Status:     in.Status,
		Fidelity:   in.Fidelity,
		Detections: in.Detections,
	})
	if err != nil {
		return tool.Errorf("failed to update coverage: %v", err), nil
	}
	t.RC.markCoverageUpdated()
	return tool.Text("coverage updated (" + id + ") for " + in.Technique + ": " + in.Status), nil
}
