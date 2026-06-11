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
// is unsupported.
type StoreHunt struct {
	RC *RunContext
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
			"hypothesis":     map[string]any{"type": "string", "description": "The hypothesis this hunt tested."},
			"outcome":        map[string]any{"type": "string", "enum": []string{brain.HuntConfirmed, brain.HuntRefuted, brain.HuntInconclusive}, "description": "Whether the hypothesis was confirmed, refuted, or left inconclusive."},
			"detection_tier": map[string]any{"type": "string", "enum": []string{brain.TierAutomated, brain.TierTriage, brain.TierRecurring, brain.TierPlaybook, brain.TierNoDetection}, "description": "The detection-output decision (highest-fidelity output this finding supports): tier1_automated (high-fidelity Sigma), tier2_triage (lower-fidelity Sigma), tier3_recurring_hunt, tier4_playbook, tier5_none_documented (justified no-build)."},
			"tier_rationale": map[string]any{"type": "string", "description": "Why this tier: what the finding does or does not support. Required for tier5; recommended for all."},
			"findings":       claimSchema,
			"hypotheses":     claimSchema,
			"next_steps":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		Required: []string{"outcome", "findings", "detection_tier"},
	}
}

func (t *StoreHunt) Run(ctx context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Hypothesis    string        `json:"hypothesis"`
		Outcome       string        `json:"outcome"`
		DetectionTier string        `json:"detection_tier"`
		TierRationale string        `json:"tier_rationale"`
		Findings      []brain.Claim `json:"findings"`
		Hypotheses    []brain.Claim `json:"hypotheses"`
		NextSteps     []string      `json:"next_steps"`
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
		HuntID:            t.RC.HuntID,
		Question:          t.RC.HuntQuestion,
		Hypothesis:        in.Hypothesis,
		Status:            in.Outcome,
		Findings:          in.Findings,
		Hypotheses:        in.Hypotheses,
		NextSteps:         in.NextSteps,
		Evidence:          t.RC.Evidence(),
		HuntType:          t.RC.HuntType(),
		DetectionTier:     in.DetectionTier,
		TierRationale:     in.TierRationale,
		DataPlanValidated: t.RC.DataPlanValidated(),
		Gaps:              t.RC.Gaps(),
		CoverageUpdated:   t.RC.CoverageUpdated(),
	}

	// Enforce the full PEAK gate: citation discipline, validate-before-query, a
	// justified detection-tier decision, and a completed feedback stage.
	if violations := brain.LintHunt(page); len(violations) > 0 {
		return tool.Errorf("hunt rejected — fix these and call store_hunt again:\n- %s", strings.Join(violations, "\n- ")), nil
	}

	summary := summarizeFindings(in.Findings)
	if err := t.RC.Brain.CloseHunt(ctx, t.RC.HuntID, in.Outcome, summary, in.DetectionTier, in.TierRationale); err != nil {
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
	msg := "hunt stored (" + in.Outcome + ", " + in.DetectionTier + "): " + url
	switch in.DetectionTier {
	case brain.TierAutomated, brain.TierTriage:
		// Tiers 1–2 convert to a Sigma rule — high-fidelity or triage-grade.
		msg += "\n\nConvert: author a Sigma rule for the proven behavior (with falsepositives, an inline runbook, and tests), validate and test it, then link it to this hunt with link_artifacts. A tier2 rule should say in its description that it surfaces candidates for review."
	case brain.TierRecurring:
		msg += "\n\nConvert: no rule is feasible yet. Queue this hunt to re-run on a cadence with queue_hunt, capturing the query so it is reproducible."
	case brain.TierPlaybook:
		msg += "\n\nConvert: automation is premature. Write the investigation method and queries as a playbook (ntn or a file) and link it so future hunts can reuse it."
	case brain.TierNoDetection:
		msg += "\n\nConvert: no detection — the rationale recorded is the deliverable. If a visibility gap blocked this hunt, queue a forensic-readiness follow-up with queue_hunt."
	}
	msg += "\n\nFeed back: call update_coverage to record this technique's coverage state, and queue any follow-on hypotheses with queue_hunt."
	return tool.Text(msg), nil
}

func summarizeFindings(findings []brain.Claim) string {
	var parts []string
	for _, f := range findings {
		parts = append(parts, f.Text)
	}
	return strings.Join(parts, "; ")
}
