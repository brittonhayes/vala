package brain

import (
	"context"
	"testing"
)

func TestBacklogLifecycle(t *testing.T) {
	ctx := context.Background()
	mem := NewMem()
	c := New(mem)

	id, err := c.QueueHunt(ctx, BacklogItem{
		Trigger:    "CISA advisory on GuardDuty tampering",
		Hypothesis: "an attacker disabled GuardDuty in the last 24h",
		Behavior:   "DeleteDetector / disable GuardDuty",
		DataSource: "cloudtrail",
		Priority:   "high",
	})
	if err != nil {
		t.Fatalf("QueueHunt: %v", err)
	}
	rows := mem.RowsIn(DBBacklog)
	if len(rows) != 1 {
		t.Fatalf("expected 1 backlog row, got %d", len(rows))
	}
	if got := rows[0].Props["status"]; got != BacklogQueued {
		t.Fatalf("new backlog item should be %q, got %v", BacklogQueued, got)
	}
	if got := rows[0].Props["behavior"]; got != "DeleteDetector / disable GuardDuty" {
		t.Fatalf("behavior not stored: %v", got)
	}

	// Opening the item into a hunt retires it and links the hunt.
	huntID, err := c.OpenHunt(ctx, Hunt{Question: "did anyone disable GuardDuty?"})
	if err != nil {
		t.Fatalf("OpenHunt: %v", err)
	}
	if err := c.SetBacklogStatus(ctx, id, BacklogOpened, huntID); err != nil {
		t.Fatalf("SetBacklogStatus: %v", err)
	}
	row := mem.Rows[id]
	if row.Props["status"] != BacklogOpened {
		t.Fatalf("backlog status = %v, want %q", row.Props["status"], BacklogOpened)
	}
	if got, _ := row.Props["hunt"].([]string); len(got) != 1 || got[0] != huntID {
		t.Fatalf("backlog.hunt = %v, want [%s]", row.Props["hunt"], huntID)
	}
}
