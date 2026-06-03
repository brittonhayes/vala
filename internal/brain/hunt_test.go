package brain

import (
	"context"
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

	if err := c.CloseHunt(ctx, huntID, HuntConfirmed, "GuardDuty was disabled"); err != nil {
		t.Fatalf("CloseHunt: %v", err)
	}
	if got := mem.Rows[huntID].Props["status"]; got != HuntConfirmed {
		t.Fatalf("closed hunt should be %q, got %v", HuntConfirmed, got)
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
