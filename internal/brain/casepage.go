package brain

import (
	"fmt"
	"strings"
)

// Evidence is an immutable pointer backing a claim: a query ID, URL, file hash,
// or log reference — never free-form prose.
type Evidence struct {
	ID         string `json:"id"`
	Claim      string `json:"claim"`
	Source     string `json:"source"`  // query | url | file_hash | log_ref
	Pointer    string `json:"pointer"` // the actual query/URL/hash
	Confidence string `json:"confidence"`
}

// Action is a proposed/approved/executed operation.
type Action struct {
	ID        string   `json:"id"`
	Class     string   `json:"class"`
	Params    string   `json:"params"`
	Rationale string   `json:"rationale"`
	Status    string   `json:"status"`
	Evidence  []string `json:"evidence"`
}

// Claim is one declarative statement in the narrative. A claim is valid only if
// it cites at least one Evidence row or is explicitly marked as a hypothesis.
type Claim struct {
	Text       string   `json:"text" yaml:"text"`
	Evidence   []string `json:"evidence" yaml:"evidence"`
	Hypothesis bool     `json:"hypothesis" yaml:"hypothesis"`
	Confidence string   `json:"confidence" yaml:"confidence"`
}

// TimelineItem is a timestamped event in the case timeline.
type TimelineItem struct {
	When     string   `json:"when" yaml:"when"`
	Text     string   `json:"text" yaml:"text"`
	Evidence []string `json:"evidence" yaml:"evidence"`
}

// CasePage is the structured narrative generated per run. Render turns it into
// the markdown the case page is written from; LintCasePage enforces that every
// declarative claim is evidence-backed.
type CasePage struct {
	CaseID       string
	Summary      []Claim
	Timeline     []TimelineItem
	EvidenceRows []Evidence
	Hypotheses   []Claim
	NextSteps    []string
	Actions      []Action
}

// Render produces the case-page markdown with the standard section skeleton.
func (p CasePage) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Case %s\n\n", p.CaseID)

	b.WriteString("## Summary\n\n")
	for _, c := range p.Summary {
		b.WriteString("- " + renderClaim(c) + "\n")
	}

	b.WriteString("\n## Timeline\n\n")
	for _, t := range p.Timeline {
		fmt.Fprintf(&b, "- %s — %s%s\n", t.When, t.Text, citeSuffix(t.Evidence))
	}

	b.WriteString("\n## Evidence\n\n")
	b.WriteString("| ID | Kind | Pointer | Confidence |\n|---|---|---|---|\n")
	for _, e := range p.EvidenceRows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", e.ID, e.Source, e.Pointer, e.Confidence)
	}

	b.WriteString("\n## Hypotheses\n\n")
	for _, c := range p.Hypotheses {
		b.WriteString("- " + renderClaim(c) + "\n")
	}

	b.WriteString("\n## Recommended next steps\n\n")
	for _, s := range p.NextSteps {
		b.WriteString("- [ ] " + s + "\n")
	}

	b.WriteString("\n## Actions taken\n\n")
	if len(p.Actions) == 0 {
		b.WriteString("_None._\n")
	}
	for _, a := range p.Actions {
		fmt.Fprintf(&b, "- **%s** (%s): %s\n", a.Class, a.Status, a.Rationale)
	}
	return b.String()
}

func renderClaim(c Claim) string {
	text := c.Text
	if c.Confidence != "" {
		text += fmt.Sprintf(" (confidence: %s)", c.Confidence)
	}
	if c.Hypothesis {
		return text + " [hypothesis]"
	}
	return text + citeSuffix(c.Evidence)
}

func citeSuffix(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return " [" + strings.Join(ids, ", ") + "]"
}

// LintCasePage returns a list of violations: declarative (non-hypothesis) claims
// in the Summary or Hypotheses sections that cite no evidence, or that cite an
// evidence ID with no matching Evidence row. This is the code-enforced version
// of the F1 acceptance criterion ("every claim is backed by an Evidence row or
// clearly marked as hypothesis").
func LintCasePage(p CasePage) []string {
	known := map[string]bool{}
	for _, e := range p.EvidenceRows {
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
	check("summary", p.Summary)
	check("hypothesis", p.Hypotheses)
	return violations
}
