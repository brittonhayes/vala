package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed validate_data.md
var validateDataDescription string

// ValidateData is the data-validation stage of the loop: before querying, the
// agent confirms the telemetry needed to test the hypothesis exists and is
// complete enough. A pass records a data_plan finding; a failed check records a
// visibility_gap finding (never a silent skip) and offers a forensic-readiness
// follow-up. Both are immutable Evidence rows linked to the active hunt.
type ValidateData struct{ RC *RunContext }

func (t *ValidateData) Name() string        { return "validate_data" }
func (t *ValidateData) Description() string { return validateDataDescription }
func (t *ValidateData) ReadOnly() bool      { return false }

func (t *ValidateData) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"sources":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "The data sources the hypothesis needs, e.g. [cloudtrail, guardduty]."},
			"time_window":  map[string]any{"type": "string", "description": "The time window the hunt covers, e.g. 'last 90 days'."},
			"completeness": map[string]any{"type": "string", "description": "What is known about the data's completeness for this window."},
			"retention":    map[string]any{"type": "string", "description": "Retention notes: does the data still exist for the window you need?"},
			"validated":    map[string]any{"type": "boolean", "description": "True if the telemetry is present and complete enough to test the hypothesis."},
			"gap":          map[string]any{"type": "string", "description": "If a telemetry check failed, the visibility gap: what is missing or incomplete. Set this (or validated:false) to record a gap instead of a plan."},
		},
		Required: []string{"sources", "validated"},
	}
}

func (t *ValidateData) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Sources      []string `json:"sources"`
		TimeWindow   string   `json:"time_window"`
		Completeness string   `json:"completeness"`
		Retention    string   `json:"retention"`
		Validated    bool     `json:"validated"`
		Gap          string   `json:"gap"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if t.RC.HuntID == "" {
		return tool.Errorf("no active hunt — call open_hunt first"), nil
	}
	if len(in.Sources) == 0 {
		return tool.Errorf("name at least one data source the hypothesis needs"), nil
	}

	// A failed check is a recorded visibility gap, not a skipped step.
	if !in.Validated || strings.TrimSpace(in.Gap) != "" {
		claim := in.Gap
		if claim == "" {
			claim = fmt.Sprintf("telemetry not validated for %s", strings.Join(in.Sources, ", "))
		}
		e := brain.Evidence{Claim: claim, Source: brain.EvidenceGap, Pointer: strings.Join(in.Sources, ", "), Confidence: "confirmed"}
		id, err := t.RC.Brain.RecordFinding(ctx, t.RC.HuntID, e)
		if err != nil {
			return tool.Errorf("failed to record visibility gap: %v", err), nil
		}
		e.ID = id
		t.RC.addGap(e)
		return tool.Text("recorded visibility gap " + id + " — this hunt is blind here. Either pivot to a data source you do have, or close with detection_tier tier5_none_documented and queue a forensic-readiness follow-up (queue_hunt) to get this telemetry."), nil
	}

	e := brain.Evidence{
		Claim:      fmt.Sprintf("telemetry validated: %s", strings.Join(in.Sources, ", ")),
		Source:     brain.EvidenceDataPlan,
		Pointer:    summarizeDataPlan(in.Sources, in.TimeWindow, in.Completeness, in.Retention),
		Confidence: "confirmed",
	}
	id, err := t.RC.Brain.RecordFinding(ctx, t.RC.HuntID, e)
	if err != nil {
		return tool.Errorf("failed to record data plan: %v", err), nil
	}
	e.ID = id
	t.RC.addEvidence(e)
	t.RC.markDataPlanValidated()
	return tool.Text("data plan validated (" + id + ") — telemetry is present. Proceed to query and record_finding."), nil
}

// summarizeDataPlan renders the data plan as a compact, single-line pointer.
func summarizeDataPlan(sources []string, window, completeness, retention string) string {
	parts := []string{"sources=" + strings.Join(sources, "+")}
	if window != "" {
		parts = append(parts, "window="+window)
	}
	if completeness != "" {
		parts = append(parts, "completeness="+completeness)
	}
	if retention != "" {
		parts = append(parts, "retention="+retention)
	}
	return strings.Join(parts, "; ")
}
