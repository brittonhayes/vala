package brain

import (
	"context"
	"testing"
)

func TestUpsertCoverageCreatesThenUpdates(t *testing.T) {
	ctx := context.Background()
	mem := NewMem()
	c := New(mem)

	id1, err := c.UpsertCoverage(ctx, Coverage{Technique: "attack.t1562.001", Status: CoverageUncovered, Fidelity: "none"})
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	if rows := mem.RowsIn(DBCoverage); len(rows) != 1 {
		t.Fatalf("expected 1 coverage row after create, got %d", len(rows))
	}

	// Re-hunting the same technique updates the existing row rather than adding one.
	id2, err := c.UpsertCoverage(ctx, Coverage{Technique: "attack.t1562.001", Status: CoverageCovered, Fidelity: "high"})
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("upsert should reuse the row: id1=%q id2=%q", id1, id2)
	}
	rows := mem.RowsIn(DBCoverage)
	if len(rows) != 1 {
		t.Fatalf("expected 1 coverage row after update, got %d", len(rows))
	}
	if got := rows[0].Props["status"]; got != CoverageCovered {
		t.Fatalf("status not updated: got %v, want %q", got, CoverageCovered)
	}
	if got := rows[0].Props["fidelity"]; got != "high" {
		t.Fatalf("fidelity not updated: got %v, want high", got)
	}

	// A different technique gets its own row.
	if _, err := c.UpsertCoverage(ctx, Coverage{Technique: "attack.t1078.004", Status: CoverageThin}); err != nil {
		t.Fatalf("third upsert: %v", err)
	}
	if rows := mem.RowsIn(DBCoverage); len(rows) != 2 {
		t.Fatalf("expected 2 coverage rows for 2 techniques, got %d", len(rows))
	}
}
