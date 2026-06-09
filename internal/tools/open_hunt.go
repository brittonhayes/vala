package tools

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed open_hunt.md
var openHuntDescription string

// OpenHunt opens a hypothesis-driven threat hunt in the brain and makes it the
// active hunt for the session, so record_finding and store_hunt have somewhere
// to write. It is the entry point of the hunting workflow in the unified
// harness: open a hunt, investigate read-only, record findings, then store_hunt
// with a verdict.
type OpenHunt struct{ RC *RunContext }

func (t *OpenHunt) Name() string        { return "open_hunt" }
func (t *OpenHunt) Description() string { return openHuntDescription }
func (t *OpenHunt) ReadOnly() bool      { return false }

func (t *OpenHunt) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"question":    map[string]any{"type": "string", "description": "The threat question this hunt investigates."},
			"hypothesis":  map[string]any{"type": "string", "description": "The hypothesis you will test (optional; you can state it as you go)."},
			"behavior":    map[string]any{"type": "string", "description": "ABLE: the specific, testable adversary behavior/TTP this hunt looks for. Strongly recommended."},
			"data_source": map[string]any{"type": "string", "description": "ABLE: the data source (Location) you will hunt in, e.g. cloudtrail. Strongly recommended."},
			"mitre":       map[string]any{"type": "string", "description": "Related MITRE ATT&CK technique, e.g. attack.t1562.001 (optional)."},
			"backlog_id":  map[string]any{"type": "string", "description": "If this hunt came off the backlog, the queue_hunt id to mark Opened and link."},
		},
		Required: []string{"question"},
	}
}

func (t *OpenHunt) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Question   string `json:"question"`
		Hypothesis string `json:"hypothesis"`
		Behavior   string `json:"behavior"`
		DataSource string `json:"data_source"`
		MITRE      string `json:"mitre"`
		BacklogID  string `json:"backlog_id"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Question == "" {
		return tool.Errorf("question is required"), nil
	}
	huntID, err := t.RC.Brain.OpenHunt(ctx, brain.Hunt{
		Question:   in.Question,
		Hypothesis: in.Hypothesis,
		Behavior:   in.Behavior,
		DataSource: in.DataSource,
		MITRE:      in.MITRE,
	})
	if err != nil {
		return tool.Errorf("failed to open hunt: %v", err), nil
	}
	t.RC.SetHunt(huntID, in.Question)
	if in.BacklogID != "" {
		// Best-effort: retire the backlog item into this hunt. A failure here must
		// not fail the hunt, which is already open.
		_ = t.RC.Brain.SetBacklogStatus(ctx, in.BacklogID, brain.BacklogOpened, huntID)
	}
	return tool.Text("opened hunt " + huntID + " — investigate read-only, record each fact with record_finding, then call store_hunt with a verdict; on Confirmed, author a Sigma detection for the behavior and link it"), nil
}
