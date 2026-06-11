package brain

import (
	"context"
	"fmt"
	"strings"
)

// Hunt is a hypothesis-driven threat hunt: a question, the hypothesis it tests,
// and its outcome. Findings are recorded as immutable Evidence rows linked back
// to the hunt, so every claim in the hunt page traces to a pointer.
type Hunt struct {
	ID         string `json:"id"`
	Question   string `json:"question"`
	Hypothesis string `json:"hypothesis"`
	Status     string `json:"status"`
	MITRE      string `json:"mitre"`
	// Behavior and DataSource carry the ABLE scoping of the hypothesis: the
	// testable adversary Behavior and the Location (data source) it is hunted in.
	// They are optional so a hunt can still open from a bare question.
	Behavior   string `json:"behavior"`
	DataSource string `json:"data_source"`
	// HuntType is the PEAK hunt style: hypothesis | baseline | model_assisted.
	// It defaults to hypothesis when unset.
	HuntType string `json:"hunt_type"`
	// DetectionTier and TierRationale record the detection-output decision made
	// when the hunt closes (see the Tier* constants). They are set by CloseHunt,
	// not OpenHunt.
	DetectionTier string `json:"detection_tier"`
	TierRationale string `json:"tier_rationale"`
}

// HuntPage is the structured narrative generated for a completed hunt. Render
// turns it into markdown; LintHuntPage enforces that every declarative finding
// cites an Evidence row or is explicitly a hypothesis.
type HuntPage struct {
	HuntID     string
	Question   string
	Hypothesis string
	Status     string
	Findings   []Claim
	Timeline   []TimelineItem
	Evidence   []Evidence
	Hypotheses []Claim
	NextSteps  []string
	// PEAK stage state. HuntType is the hunt style; DetectionTier and
	// TierRationale are the Document-&-Decide deliverable; DataPlanValidated,
	// Gaps, and CoverageUpdated record whether the data-validation and feedback
	// stages ran. LintHunt enforces the invariants over these.
	HuntType          string
	DetectionTier     string
	TierRationale     string
	DataPlanValidated bool
	Gaps              []Evidence
	CoverageUpdated   bool
}

// Render produces the hunt-page markdown with the standard section skeleton.
func (p HuntPage) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Hunt %s\n\n", p.HuntID)

	b.WriteString("## Question\n\n")
	b.WriteString(p.Question + "\n")

	b.WriteString("\n## Hypothesis\n\n")
	b.WriteString(p.Hypothesis + "\n")

	if p.Status != "" {
		fmt.Fprintf(&b, "\n**Outcome:** %s\n", p.Status)
	}
	if p.HuntType != "" {
		fmt.Fprintf(&b, "**Hunt type:** %s\n", p.HuntType)
	}

	b.WriteString("\n## Findings\n\n")
	for _, c := range p.Findings {
		b.WriteString("- " + renderClaim(c) + "\n")
	}

	b.WriteString("\n## Timeline\n\n")
	for _, t := range p.Timeline {
		fmt.Fprintf(&b, "- %s — %s%s\n", t.When, t.Text, citeSuffix(t.Evidence))
	}

	b.WriteString("\n## Evidence\n\n")
	b.WriteString("| ID | Kind | Pointer | Confidence |\n|---|---|---|---|\n")
	for _, e := range p.Evidence {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", e.ID, e.Source, e.Pointer, e.Confidence)
	}

	if p.DataPlanValidated || len(p.Gaps) > 0 {
		b.WriteString("\n## Data plan & visibility gaps\n\n")
		if p.DataPlanValidated {
			b.WriteString("- Telemetry validated before execution.\n")
		}
		for _, g := range p.Gaps {
			fmt.Fprintf(&b, "- Visibility gap: %s%s\n", g.Claim, citeSuffix([]string{g.ID}))
		}
	}

	if p.DetectionTier != "" {
		b.WriteString("\n## Detection decision\n\n")
		fmt.Fprintf(&b, "**Tier:** %s\n", p.DetectionTier)
		if p.TierRationale != "" {
			fmt.Fprintf(&b, "\n%s\n", p.TierRationale)
		}
	}

	if p.CoverageUpdated {
		b.WriteString("\n## Coverage impact\n\n")
		b.WriteString("- Coverage map updated for this hunt's technique.\n")
	}

	b.WriteString("\n## Open hypotheses\n\n")
	for _, c := range p.Hypotheses {
		b.WriteString("- " + renderClaim(c) + "\n")
	}

	b.WriteString("\n## Recommended next steps\n\n")
	for _, s := range p.NextSteps {
		b.WriteString("- [ ] " + s + "\n")
	}
	return b.String()
}

// LintHuntPage returns a list of violations: declarative (non-hypothesis) claims
// in the Findings or Hypotheses sections that cite no evidence, or that cite an
// evidence ID with no matching Evidence row. This is the code-enforced version
// of the rule that every hunt finding is backed by an immutable pointer.
func LintHuntPage(p HuntPage) []string {
	known := map[string]bool{}
	for _, e := range p.Evidence {
		known[e.ID] = true
	}
	var violations []string
	check := func(section string, claims []Claim) {
		for _, c := range claims {
			if c.Hypothesis {
				continue
			}
			if len(c.Evidence) == 0 {
				violations = append(violations, fmt.Sprintf("%s claim has no evidence and is not marked a hypothesis: %q", section, c.Text))
				continue
			}
			for _, id := range c.Evidence {
				if !known[id] {
					violations = append(violations, fmt.Sprintf("%s claim cites unknown evidence %q: %q", section, id, c.Text))
				}
			}
		}
	}
	check("finding", p.Findings)
	check("hypothesis", p.Hypotheses)
	return violations
}

// LintHunt is the full PEAK gate run before a hunt is stored. It layers the loop
// invariants on top of the citation check (LintHuntPage): data was validated
// before it was queried, a detection-tier decision was made and justified, and
// the feedback stage left a coverage delta or a follow-on action. A hunt that
// violates any of these is not "high quality" and must not close.
func LintHunt(p HuntPage) []string {
	violations := LintHuntPage(p)

	// Stage 3 before Stage 4: if the hunt queried evidence, it must have first
	// recorded a validated data plan. A recorded visibility gap counts — a failed
	// check is a real, documented outcome, not a skipped step.
	queried := false
	for _, e := range p.Evidence {
		if e.Source == EvidenceQuery {
			queried = true
			break
		}
	}
	if queried && !p.DataPlanValidated && len(p.Gaps) == 0 {
		violations = append(violations, "queried evidence before validating data availability: call validate_data first (a failed check is recorded as a visibility gap, never skipped)")
	}

	// Stage 6: every hunt closes with a detection-output decision, and it must be
	// justified — a no-build (tier 5) most of all.
	switch p.DetectionTier {
	case "":
		violations = append(violations, "no detection-output decision: store_hunt must pick a detection_tier (tier1_automated … tier5_none_documented)")
	case TierNoDetection:
		if strings.TrimSpace(p.TierRationale) == "" {
			violations = append(violations, "a no-build decision (tier5_none_documented) must be justified: give a tier_rationale")
		}
	}
	if p.DetectionTier != "" && strings.TrimSpace(p.TierRationale) == "" {
		violations = append(violations, "detection-tier decision is unjustified: give a tier_rationale for the chosen tier")
	}

	// Stage 8: feedback must leave the coverage map updated or a follow-on action
	// queued, so each hunt compounds into the next.
	if !p.CoverageUpdated && len(p.NextSteps) == 0 {
		violations = append(violations, "feedback stage incomplete: call update_coverage and/or record at least one follow-on next step")
	}

	return violations
}

// OpenHunt creates a Hunts row in the Open state and returns its ID.
func (c *Client) OpenHunt(ctx context.Context, h Hunt) (huntID string, err error) {
	props := map[string]any{
		"hunt_id":    h.Question,
		"question":   h.Question,
		"hypothesis": h.Hypothesis,
		"status":     HuntOpen,
		"mitre":      h.MITRE,
		"started_at": nowRFC3339(),
	}
	if h.Behavior != "" {
		props["behavior"] = h.Behavior
	}
	if h.DataSource != "" {
		props["data_source"] = h.DataSource
	}
	if h.HuntType != "" {
		props["hunt_type"] = h.HuntType
	}
	return c.n.CreateRow(ctx, c.dbName(DBHunts), props)
}

// RecordFinding appends an immutable Evidence row linked to the hunt and returns
// its ID. It reuses the single Evidence store; a finding is an Evidence row
// whose `hunt` relation points at the hunt instead of a `case`.
func (c *Client) RecordFinding(ctx context.Context, huntID string, e Evidence) (string, error) {
	return c.n.CreateRow(ctx, c.dbName(DBEvidence), map[string]any{
		"hunt":         huntID,
		"claim":        e.Claim,
		"kind":         e.Source,
		"pointer":      e.Pointer,
		"confidence":   e.Confidence,
		"collected_at": nowRFC3339(),
	})
}

// CloseHunt finalizes a Hunts row with its outcome status, a findings summary,
// and the detection-output decision (tier + rationale) made for the hunt.
func (c *Client) CloseHunt(ctx context.Context, huntID, status, findings, detectionTier, tierRationale string) error {
	props := map[string]any{
		"status":   status,
		"findings": findings,
		"ended_at": nowRFC3339(),
	}
	if detectionTier != "" {
		props["detection_tier"] = detectionTier
	}
	if tierRationale != "" {
		props["tier_rationale"] = tierRationale
	}
	return c.n.UpdateRow(ctx, huntID, props)
}

// WriteHuntPage renders and creates the narrative hunt page, returning its URL.
func (c *Client) WriteHuntPage(ctx context.Context, title string, p HuntPage) (string, error) {
	_, url, err := c.n.CreatePage(ctx, title, p.Render())
	return url, err
}
