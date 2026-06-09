package brain

import (
	"context"
	"testing"
)

func TestRecordIntelAndDetection(t *testing.T) {
	ctx := context.Background()
	mem := NewMem()
	c := New(mem)

	intelID, err := c.RecordIntel(ctx, Intel{Kind: IntelTTP, Value: "attack.t1562.001", MITRE: "attack.t1562.001"})
	if err != nil {
		t.Fatalf("RecordIntel: %v", err)
	}
	if rows := mem.RowsIn(DBIntel); len(rows) != 1 {
		t.Fatalf("expected 1 intel row, got %d", len(rows))
	} else if rows[0].Props["kind"] != IntelTTP {
		t.Fatalf("intel kind = %v, want %q", rows[0].Props["kind"], IntelTTP)
	}

	detID, err := c.RecordDetection(ctx, DetectionRef{ID: "rule-1", Title: "GuardDuty disabled", Intel: []string{intelID}})
	if err != nil {
		t.Fatalf("RecordDetection: %v", err)
	}
	if rows := mem.RowsIn(DBDetections); len(rows) != 1 {
		t.Fatalf("expected 1 detection row, got %d", len(rows))
	}
	// The inline relation set at creation time should be present.
	if got, _ := mem.Rows[detID].Props["intel"].([]string); len(got) != 1 || got[0] != intelID {
		t.Fatalf("detection.intel = %v, want [%s]", mem.Rows[detID].Props["intel"], intelID)
	}
}

func TestLinkConnectsArtifacts(t *testing.T) {
	ctx := context.Background()
	mem := NewMem()
	c := New(mem)

	huntID, err := c.OpenHunt(ctx, Hunt{Question: "q"})
	if err != nil {
		t.Fatalf("OpenHunt: %v", err)
	}
	detID, err := c.RecordDetection(ctx, DetectionRef{ID: "rule-1"})
	if err != nil {
		t.Fatalf("RecordDetection: %v", err)
	}

	if err := c.Link(ctx, huntID, "detections", detID); err != nil {
		t.Fatalf("Link: %v", err)
	}
	got, ok := mem.Rows[huntID].Props["detections"].([]string)
	if !ok || len(got) != 1 || got[0] != detID {
		t.Fatalf("hunt.detections = %v, want [%s]", mem.Rows[huntID].Props["detections"], detID)
	}

	// Linking with no targets is a no-op, not an error.
	if err := c.Link(ctx, huntID, "intel"); err != nil {
		t.Fatalf("empty Link should be a no-op: %v", err)
	}
}
