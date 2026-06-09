package tools

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed link_artifacts.md
var linkArtifactsDescription string

// LinkArtifacts connects brain artifacts by setting a relation on a row to one
// or more target row IDs — the mechanism that turns the brain into a connected
// graph of intel, hunts, and detections.
type LinkArtifacts struct{ RC *RunContext }

func (t *LinkArtifacts) Name() string        { return "link_artifacts" }
func (t *LinkArtifacts) Description() string { return linkArtifactsDescription }
func (t *LinkArtifacts) ReadOnly() bool      { return false }

func (t *LinkArtifacts) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"from_id":  map[string]any{"type": "string", "description": "The row ID to set the relation on."},
			"relation": map[string]any{"type": "string", "enum": []string{"evidence", "intel", "hunts", "detections"}, "description": "The relation property to set."},
			"to_ids":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "The target row IDs to link to."},
		},
		Required: []string{"from_id", "relation", "to_ids"},
	}
}

func (t *LinkArtifacts) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		FromID   string   `json:"from_id"`
		Relation string   `json:"relation"`
		ToIDs    []string `json:"to_ids"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.FromID == "" || in.Relation == "" || len(in.ToIDs) == 0 {
		return tool.Errorf("from_id, relation, and at least one to_id are required"), nil
	}
	if err := t.RC.Brain.Link(ctx, in.FromID, in.Relation, in.ToIDs...); err != nil {
		return tool.Errorf("failed to link: %v", err), nil
	}
	return tool.Text("linked " + in.FromID + " " + in.Relation + " -> " + joinIDs(in.ToIDs)), nil
}

func joinIDs(ids []string) string {
	out := ""
	for i, id := range ids {
		if i > 0 {
			out += ", "
		}
		out += id
	}
	return out
}
