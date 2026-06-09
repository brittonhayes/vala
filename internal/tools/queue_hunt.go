package tools

import (
	"context"
	_ "embed"
	"encoding/json"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed queue_hunt.md
var queueHuntDescription string

// QueueHunt records a trigger as a prioritized hunt hypothesis on the backlog so
// it is durable and rankable before anyone hunts it — the Scope step of the hunt
// loop. open_hunt later pulls a backlog item into an active hunt.
type QueueHunt struct{ RC *RunContext }

func (t *QueueHunt) Name() string        { return "queue_hunt" }
func (t *QueueHunt) Description() string { return queueHuntDescription }
func (t *QueueHunt) ReadOnly() bool      { return false }

func (t *QueueHunt) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"trigger":     map[string]any{"type": "string", "description": "What prompted this hunt: threat intel, a hunch, a fresh CVE, a past incident."},
			"hypothesis":  map[string]any{"type": "string", "description": "The falsifiable claim to test."},
			"behavior":    map[string]any{"type": "string", "description": "ABLE: the specific, testable adversary behavior/TTP to hunt for."},
			"data_source": map[string]any{"type": "string", "description": "ABLE: the data source (Location) the behavior would show up in, e.g. cloudtrail."},
			"priority":    map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}, "description": "Hunt priority."},
			"mitre":       map[string]any{"type": "string", "description": "Related MITRE ATT&CK technique, e.g. attack.t1562.001 (optional)."},
		},
		Required: []string{"trigger", "hypothesis"},
	}
}

func (t *QueueHunt) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Trigger    string `json:"trigger"`
		Hypothesis string `json:"hypothesis"`
		Behavior   string `json:"behavior"`
		DataSource string `json:"data_source"`
		Priority   string `json:"priority"`
		MITRE      string `json:"mitre"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Trigger == "" || in.Hypothesis == "" {
		return tool.Errorf("trigger and hypothesis are required"), nil
	}
	id, err := t.RC.Brain.QueueHunt(ctx, brain.BacklogItem{
		Trigger:    in.Trigger,
		Hypothesis: in.Hypothesis,
		Behavior:   in.Behavior,
		DataSource: in.DataSource,
		Priority:   in.Priority,
		MITRE:      in.MITRE,
	})
	if err != nil {
		return tool.Errorf("failed to queue hunt: %v", err), nil
	}
	return tool.Text("queued hunt " + id + " — open it with open_hunt (pass backlog_id=" + id + ") when you start hunting"), nil
}
