package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"strings"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed write_case_page.md
var writeCasePageDescription string

// WriteCasePage composes and writes the narrative case page. The model supplies
// structured claims (each citing evidence IDs or flagged as a hypothesis); the
// tool fills the Evidence table and Actions from the run's collected state, lints
// the page, and refuses to write if any claim is unsupported. Class: case_write.
type WriteCasePage struct{ RC *RunContext }

func (t *WriteCasePage) Name() string        { return "write_case_page" }
func (t *WriteCasePage) Description() string { return writeCasePageDescription }
func (t *WriteCasePage) ReadOnly() bool      { return false }

func (t *WriteCasePage) Schema() tool.Schema {
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
			"summary":    claimSchema,
			"hypotheses": claimSchema,
			"timeline": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"when":     map[string]any{"type": "string"},
						"text":     map[string]any{"type": "string"},
						"evidence": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					},
				},
			},
			"next_steps": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		Required: []string{"summary"},
	}
}

func (t *WriteCasePage) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Summary    []brain.Claim        `json:"summary"`
		Hypotheses []brain.Claim        `json:"hypotheses"`
		Timeline   []brain.TimelineItem `json:"timeline"`
		NextSteps  []string             `json:"next_steps"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}

	page := brain.CasePage{
		CaseID:       t.RC.CaseID,
		Summary:      in.Summary,
		Hypotheses:   in.Hypotheses,
		Timeline:     in.Timeline,
		NextSteps:    in.NextSteps,
		EvidenceRows: t.RC.Evidence(),
		Actions:      t.RC.Actions(),
	}

	// Enforce the F1 invariant: every declarative claim must be evidence-backed.
	if violations := brain.LintCasePage(page); len(violations) > 0 {
		return tool.Errorf("case page rejected — fix these claims and rewrite:\n- %s", strings.Join(violations, "\n- ")), nil
	}

	url, err := t.RC.Brain.WriteCasePage(ctx, t.RC.CaseID, page)
	if err != nil {
		return tool.Errorf("failed to write case page: %v", err), nil
	}
	if url == "" {
		url = "(written)"
	}
	return tool.Text("case page written: " + url), nil
}
