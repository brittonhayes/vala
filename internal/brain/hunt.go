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

// CloseHunt finalizes a Hunts row with its outcome status and a findings summary.
func (c *Client) CloseHunt(ctx context.Context, huntID, status, findings string) error {
	return c.n.UpdateRow(ctx, huntID, map[string]any{
		"status":   status,
		"findings": findings,
		"ended_at": nowRFC3339(),
	})
}

// WriteHuntPage renders and creates the narrative hunt page, returning its URL.
func (c *Client) WriteHuntPage(ctx context.Context, title string, p HuntPage) (string, error) {
	_, url, err := c.n.CreatePage(ctx, title, p.Render())
	return url, err
}
