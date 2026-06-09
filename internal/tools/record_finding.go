package tools

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed record_finding.md
var recordFindingDescription string

// RecordFinding appends an immutable Evidence row to the current hunt and returns
// its ID for the model to cite in the hunt's findings. A finding is an Evidence
// row whose pointer (a query ID, URL, hash, or log reference) backs a claim,
// linked to the active hunt.
type RecordFinding struct{ RC *RunContext }

func (t *RecordFinding) Name() string        { return "record_finding" }
func (t *RecordFinding) Description() string { return recordFindingDescription }
func (t *RecordFinding) ReadOnly() bool      { return false }

func (t *RecordFinding) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"claim":      map[string]any{"type": "string", "description": "The specific factual finding this evidence supports."},
			"source":     map[string]any{"type": "string", "enum": []string{"query", "url", "file_hash", "log_ref"}, "description": "The kind of pointer."},
			"pointer":    map[string]any{"type": "string", "description": "The immutable pointer: a query ID/string, URL, hash, or log reference. Not prose."},
			"confidence": map[string]any{"type": "string", "enum": []string{"confirmed", "probable", "hypothesis"}, "description": "Confidence in the finding."},
		},
		Required: []string{"claim", "source", "pointer"},
	}
}

func (t *RecordFinding) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Claim, Source, Pointer, Confidence string
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Claim == "" || in.Pointer == "" {
		return tool.Errorf("claim and pointer are required"), nil
	}
	if t.RC.HuntID == "" {
		return tool.Errorf("no active hunt"), nil
	}
	if in.Confidence == "" {
		in.Confidence = "probable"
	}
	e := brain.Evidence{Claim: in.Claim, Source: in.Source, Pointer: in.Pointer, Confidence: in.Confidence}
	id, err := t.RC.Brain.RecordFinding(ctx, t.RC.HuntID, e)
	if err != nil {
		return tool.Errorf("failed to write finding: %v", err), nil
	}
	e.ID = id
	t.RC.addEvidence(e)
	return tool.Text("recorded finding " + id + " — cite this ID in the hunt's findings"), nil
}
