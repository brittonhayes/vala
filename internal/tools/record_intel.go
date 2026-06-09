package tools

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed record_intel.md
var recordIntelDescription string

// RecordIntel stores a piece of threat intelligence as a first-class brain
// artifact and returns its ID. When run inside a hunt, the intel is linked back
// to the hunt automatically.
type RecordIntel struct{ RC *RunContext }

func (t *RecordIntel) Name() string        { return "record_intel" }
func (t *RecordIntel) Description() string { return recordIntelDescription }
func (t *RecordIntel) ReadOnly() bool      { return false }

func (t *RecordIntel) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"kind":        map[string]any{"type": "string", "enum": []string{brain.IntelIndicator, brain.IntelTTP, brain.IntelActor, brain.IntelNarrative}, "description": "The kind of intelligence."},
			"value":       map[string]any{"type": "string", "description": "The IOC, technique ID, actor name, or short title."},
			"mitre":       map[string]any{"type": "string", "description": "Related MITRE ATT&CK technique, e.g. attack.t1562.001 (optional)."},
			"confidence":  map[string]any{"type": "string", "enum": []string{"confirmed", "probable", "hypothesis"}, "description": "Confidence in the intel."},
			"source":      map[string]any{"type": "string", "description": "Where the intel came from (a report, a finding ID, a URL)."},
			"description": map[string]any{"type": "string", "description": "A short description of the intelligence."},
		},
		Required: []string{"kind", "value"},
	}
}

func (t *RecordIntel) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Kind, Value, MITRE, Confidence, Source, Description string
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Kind == "" || in.Value == "" {
		return tool.Errorf("kind and value are required"), nil
	}
	intel := brain.Intel{
		Kind: in.Kind, Value: in.Value, MITRE: in.MITRE,
		Confidence: in.Confidence, Source: in.Source, Description: in.Description,
	}
	if t.RC.HuntID != "" {
		intel.Hunts = []string{t.RC.HuntID}
	}
	id, err := t.RC.Brain.RecordIntel(ctx, intel)
	if err != nil {
		return tool.Errorf("failed to record intel: %v", err), nil
	}
	return tool.Text("recorded intel " + id + " — link it to hunts or detections with link_artifacts"), nil
}
