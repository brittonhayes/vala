package brain

import "testing"

func TestLintCasePageFlagsUnsupportedClaims(t *testing.T) {
	page := CasePage{
		EvidenceRows: []Evidence{{ID: "evidence_0001"}},
		Summary: []Claim{
			{Text: "Backed claim", Evidence: []string{"evidence_0001"}},
			{Text: "Unsupported claim", Evidence: nil},
			{Text: "A guess", Hypothesis: true},
			{Text: "Cites missing evidence", Evidence: []string{"evidence_9999"}},
		},
	}
	v := LintCasePage(page)
	if len(v) != 2 {
		t.Fatalf("expected 2 violations (unsupported + missing-evidence), got %d: %v", len(v), v)
	}
}

func TestLintCasePageClean(t *testing.T) {
	page := CasePage{
		EvidenceRows: []Evidence{{ID: "e1"}},
		Summary:      []Claim{{Text: "ok", Evidence: []string{"e1"}}},
		Hypotheses:   []Claim{{Text: "maybe", Hypothesis: true}},
	}
	if v := LintCasePage(page); len(v) != 0 {
		t.Fatalf("expected clean page, got violations: %v", v)
	}
}

func TestCasePageRenderMarksHypotheses(t *testing.T) {
	page := CasePage{
		CaseID:     "C1",
		Hypotheses: []Claim{{Text: "could be lateral movement", Hypothesis: true}},
	}
	out := page.Render()
	if want := "[hypothesis]"; !contains(out, want) {
		t.Fatalf("rendered page should mark hypotheses with %q:\n%s", want, out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
