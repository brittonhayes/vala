package tools

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed record_evidence.md
var recordEvidenceDescription string

// RecordEvidence appends an immutable Evidence row to the case brain and returns
// its ID for the model to cite in later claims and action proposals. Class:
// case_write (available during Evidence/Report, never an action).
type RecordEvidence struct{ RC *RunContext }

func (t *RecordEvidence) Name() string        { return "record_evidence" }
func (t *RecordEvidence) Description() string { return recordEvidenceDescription }
func (t *RecordEvidence) ReadOnly() bool      { return false }

func (t *RecordEvidence) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"claim":      map[string]any{"type": "string", "description": "The specific factual claim this evidence supports."},
			"source":     map[string]any{"type": "string", "enum": []string{"query", "url", "file_hash", "log_ref"}, "description": "The kind of pointer."},
			"pointer":    map[string]any{"type": "string", "description": "The immutable pointer: a query ID/string, URL, hash, or log reference. Not prose."},
			"confidence": map[string]any{"type": "string", "enum": []string{"confirmed", "probable", "hypothesis"}, "description": "Confidence in the claim."},
		},
		Required: []string{"claim", "source", "pointer"},
	}
}

func (t *RecordEvidence) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Claim, Source, Pointer, Confidence string
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Claim == "" || in.Pointer == "" {
		return tool.Errorf("claim and pointer are required"), nil
	}
	if in.Confidence == "" {
		in.Confidence = "probable"
	}
	e := brain.Evidence{Claim: in.Claim, Source: in.Source, Pointer: in.Pointer, Confidence: in.Confidence}
	id, err := t.RC.Brain.RecordEvidence(ctx, t.RC.CaseID, e)
	if err != nil {
		return tool.Errorf("failed to write evidence: %v", err), nil
	}
	e.ID = id
	t.RC.addEvidence(e)
	return tool.Text("recorded evidence " + id + " — cite this ID in claims and action proposals"), nil
}
