package brain

import (
	"fmt"
	"strings"
)

// Evidence is an immutable pointer backing a claim: a query ID, URL, file hash,
// or log reference — never free-form prose. It is the row a hunt finding is
// recorded as, linked back to the hunt it supports.
type Evidence struct {
	ID         string `json:"id"`
	Claim      string `json:"claim"`
	Source     string `json:"source"`  // query | url | file_hash | log_ref
	Pointer    string `json:"pointer"` // the actual query/URL/hash
	Confidence string `json:"confidence"`
}

// Claim is one declarative statement in a narrative page. A claim is valid only
// if it cites at least one Evidence row or is explicitly marked as a hypothesis.
type Claim struct {
	Text       string   `json:"text" yaml:"text"`
	Evidence   []string `json:"evidence" yaml:"evidence"`
	Hypothesis bool     `json:"hypothesis" yaml:"hypothesis"`
	Confidence string   `json:"confidence" yaml:"confidence"`
}

// TimelineItem is a timestamped event in a narrative timeline.
type TimelineItem struct {
	When     string   `json:"when" yaml:"when"`
	Text     string   `json:"text" yaml:"text"`
	Evidence []string `json:"evidence" yaml:"evidence"`
}

// renderClaim renders a claim as a markdown bullet, appending its confidence and
// either a [hypothesis] marker or its cited evidence IDs.
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
