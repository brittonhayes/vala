package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed store_hunt.md
var storeHuntDescription string

// StoreHunt composes and writes the narrative hunt page, then closes the hunt
// with its outcome. The model supplies structured findings (each citing finding
// IDs or flagged as a hypothesis); the tool fills the Evidence table from the
// run's recorded findings, lints the page, and refuses to write if any finding
// is unsupported. Class: case_write.
type StoreHunt struct {
	RC       *RunContext
	Question string
}

func (t *StoreHunt) Name() string        { return "store_hunt" }
func (t *StoreHunt) Description() string { return storeHuntDescription }
func (t *StoreHunt) ReadOnly() bool      { return false }

func (t *StoreHunt) Schema() tool.Schema {
	claimSchema := map[string]any{
		"type": "array",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":       map[string]any{"type": "string"},
				"evidence":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				"hypothesis": map[string]any{"type": "boolean"},
				"confidence": map[string]any{"type": "string"},
			},
			"required": []string{"text"},
		},
	}
	return tool.Schema{
		Properties: map[string]any{
			"hypothesis": map[string]any{"type": "string", "description": "The hypothesis this hunt tested."},
			"outcome":    map[string]any{"type": "string", "enum": []string{brain.HuntConfirmed, brain.HuntRefuted, brain.HuntInconclusive}, "description": "Whether the hypothesis was confirmed, refuted, or left inconclusive."},
			"findings":   claimSchema,
			"hypotheses": claimSchema,
			"next_steps": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		Required: []string{"outcome", "findings"},
	}
}

func (t *StoreHunt) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Hypothesis string        `json:"hypothesis"`
		Outcome    string        `json:"outcome"`
		Findings   []brain.Claim `json:"findings"`
		Hypotheses []brain.Claim `json:"hypotheses"`
		NextSteps  []string      `json:"next_steps"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if t.RC.HuntID == "" {
		return tool.Errorf("no active hunt"), nil
	}
	switch in.Outcome {
	case brain.HuntConfirmed, brain.HuntRefuted, brain.HuntInconclusive:
	default:
		return tool.Errorf("outcome must be one of %s, %s, %s", brain.HuntConfirmed, brain.HuntRefuted, brain.HuntInconclusive), nil
	}

	page := brain.HuntPage{
		HuntID:     t.RC.HuntID,
		Question:   t.Question,
		Hypothesis: in.Hypothesis,
		Status:     in.Outcome,
		Findings:   in.Findings,
		Hypotheses: in.Hypotheses,
		NextSteps:  in.NextSteps,
		Evidence:   t.RC.Evidence(),
	}

	// Enforce the evidence invariant: every declarative finding must be backed.
	if violations := brain.LintHuntPage(page); len(violations) > 0 {
		return tool.Errorf("hunt page rejected — fix these findings and rewrite:\n- %s", strings.Join(violations, "\n- ")), nil
	}

	summary := summarizeFindings(in.Findings)
	if err := t.RC.Brain.CloseHunt(ctx, t.RC.HuntID, in.Outcome, summary); err != nil {
		return tool.Errorf("failed to close hunt: %v", err), nil
	}
	url, err := t.RC.Brain.WriteHuntPage(ctx, t.RC.HuntID, page)
	if err != nil {
		return tool.Errorf("failed to write hunt page: %v", err), nil
	}
	t.RC.setHuntOutcome(in.Outcome, url)
	if url == "" {
		url = "(written)"
	}
	return tool.Text("hunt stored (" + in.Outcome + "): " + url), nil
}

func summarizeFindings(findings []brain.Claim) string {
	var parts []string
	for _, f := range findings {
		parts = append(parts, f.Text)
	}
	return strings.Join(parts, "; ")
}
