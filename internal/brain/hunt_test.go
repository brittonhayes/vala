package brain

import (
	"context"
	"strings"
	"testing"
)

func TestHuntLifecycle(t *testing.T) {
	ctx := context.Background()
	mem := NewMem()
	c := New(mem)

	huntID, err := c.OpenHunt(ctx, Hunt{Question: "did anyone disable GuardDuty?"})
	if err != nil {
		t.Fatalf("OpenHunt: %v", err)
	}
	if rows := mem.RowsIn(DBHunts); len(rows) != 1 {
		t.Fatalf("expected 1 hunt row, got %d", len(rows))
	} else if rows[0].Props["status"] != HuntOpen {
		t.Fatalf("new hunt should be %q, got %v", HuntOpen, rows[0].Props["status"])
	}

	fid, err := c.RecordFinding(ctx, huntID, Evidence{Claim: "DeleteDetector seen", Source: "log_ref", Pointer: "q-1"})
	if err != nil {
		t.Fatalf("RecordFinding: %v", err)
	}
	ev := mem.RowsIn(DBEvidence)
	if len(ev) != 1 {
		t.Fatalf("expected 1 evidence row, got %d", len(ev))
	}
	if ev[0].Props["hunt"] != huntID {
		t.Fatalf("finding should link to hunt %q, got %v", huntID, ev[0].Props["hunt"])
	}
	if fid == "" {
		t.Fatal("RecordFinding returned empty id")
	}

	if err := c.CloseHunt(ctx, huntID, HuntConfirmed, "GuardDuty was disabled", TierAutomated, "clean, high-fidelity signal"); err != nil {
		t.Fatalf("CloseHunt: %v", err)
	}
	if got := mem.Rows[huntID].Props["status"]; got != HuntConfirmed {
		t.Fatalf("closed hunt should be %q, got %v", HuntConfirmed, got)
	}
	if got := mem.Rows[huntID].Props["detection_tier"]; got != TierAutomated {
		t.Fatalf("closed hunt should record tier %q, got %v", TierAutomated, got)
	}
}

func TestLintHuntPageFlagsUnsupportedFindings(t *testing.T) {
	page := HuntPage{
		Evidence: []Evidence{{ID: "e1"}},
		Findings: []Claim{
			{Text: "backed", Evidence: []string{"e1"}},
			{Text: "unsupported"},
			{Text: "a guess", Hypothesis: true},
			{Text: "missing", Evidence: []string{"e9"}},
		},
	}
	if v := LintHuntPage(page); len(v) != 2 {
		t.Fatalf("expected 2 violations (unsupported + missing), got %d: %v", len(v), v)
	}
}

func TestLintHuntPageClean(t *testing.T) {
	page := HuntPage{
		Evidence:   []Evidence{{ID: "e1"}},
		Findings:   []Claim{{Text: "ok", Evidence: []string{"e1"}}},
		Hypotheses: []Claim{{Text: "maybe", Hypothesis: true}},
	}
	if v := LintHuntPage(page); len(v) != 0 {
		t.Fatalf("expected clean page, got: %v", v)
	}
}

// cleanPEAKPage is a HuntPage that satisfies every PEAK invariant, so each test
// below can flip exactly one field and assert the matching violation.
func cleanPEAKPage() HuntPage {
	return HuntPage{
		Evidence:          []Evidence{{ID: "e1", Source: EvidenceQuery}},
		Findings:          []Claim{{Text: "ok", Evidence: []string{"e1"}}},
		DetectionTier:     TierAutomated,
		TierRationale:     "clean signal",
		DataPlanValidated: true,
		CoverageUpdated:   true,
	}
}

func TestLintHuntClean(t *testing.T) {
	if v := LintHunt(cleanPEAKPage()); len(v) != 0 {
		t.Fatalf("expected clean PEAK page, got: %v", v)
	}
}

func TestLintHuntRejectsQueryBeforeValidation(t *testing.T) {
	p := cleanPEAKPage()
	p.DataPlanValidated = false // queried without validating data
	if v := LintHunt(p); !hasViolation(v, "validating data") {
		t.Fatalf("expected validate-before-query violation, got: %v", v)
	}
}

func TestLintHuntAcceptsRecordedGapInsteadOfPlan(t *testing.T) {
	p := cleanPEAKPage()
	p.DataPlanValidated = false
	p.Gaps = []Evidence{{ID: "g1", Source: EvidenceGap, Claim: "no cloudtrail"}}
	if v := LintHunt(p); hasViolation(v, "validating data") {
		t.Fatalf("a recorded gap should satisfy the data stage, got: %v", v)
	}
}

func TestLintHuntRequiresTierDecision(t *testing.T) {
	p := cleanPEAKPage()
	p.DetectionTier = ""
	if v := LintHunt(p); !hasViolation(v, "detection-output decision") {
		t.Fatalf("expected missing-tier violation, got: %v", v)
	}
}

func TestLintHuntRequiresJustifiedNoBuild(t *testing.T) {
	p := cleanPEAKPage()
	p.DetectionTier = TierNoDetection
	p.TierRationale = ""
	if v := LintHunt(p); !hasViolation(v, "no-build") {
		t.Fatalf("expected unjustified no-build violation, got: %v", v)
	}
}

func TestLintHuntRequiresFeedback(t *testing.T) {
	p := cleanPEAKPage()
	p.CoverageUpdated = false
	p.NextSteps = nil
	if v := LintHunt(p); !hasViolation(v, "feedback stage") {
		t.Fatalf("expected feedback violation, got: %v", v)
	}
}

func hasViolation(violations []string, substr string) bool {
	for _, v := range violations {
		if strings.Contains(v, substr) {
			return true
		}
	}
	return false
}
