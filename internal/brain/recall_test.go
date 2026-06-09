package brain

import (
	"context"
	"testing"
)

func TestRecallMatchesAndFilters(t *testing.T) {
	ctx := context.Background()
	bc := New(NewMem())

	// Two hunts: one about GuardDuty, one about S3.
	if _, err := bc.OpenHunt(ctx, Hunt{Question: "did anyone disable GuardDuty?", Behavior: "DeleteDetector"}); err != nil {
		t.Fatalf("OpenHunt: %v", err)
	}
	if _, err := bc.OpenHunt(ctx, Hunt{Question: "public S3 bucket exposure?", Behavior: "PutBucketAcl"}); err != nil {
		t.Fatalf("OpenHunt: %v", err)
	}

	// A query scoped to the behavior returns only the matching hunt.
	got, err := bc.Recall(ctx, DBHunts, "DeleteDetector", 5)
	if err != nil {
		t.Fatalf("Recall: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 hunt matching DeleteDetector, got %d: %+v", len(got), got)
	}
	if got[0].Props["behavior"] != "DeleteDetector" {
		t.Fatalf("wrong hunt recalled: %+v", got[0].Props)
	}

	// An empty query returns everything in the database.
	all, err := bc.Recall(ctx, DBHunts, "", 5)
	if err != nil {
		t.Fatalf("Recall all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 hunts on empty query, got %d", len(all))
	}

	// limit caps the result set.
	capped, err := bc.Recall(ctx, DBHunts, "", 1)
	if err != nil {
		t.Fatalf("Recall capped: %v", err)
	}
	if len(capped) != 1 {
		t.Fatalf("expected limit to cap at 1, got %d", len(capped))
	}

	// A non-matching query is empty, not an error.
	none, err := bc.Recall(ctx, DBHunts, "no-such-behavior", 5)
	if err != nil {
		t.Fatalf("Recall none: %v", err)
	}
	if len(none) != 0 {
		t.Fatalf("expected no matches, got %d", len(none))
	}
}
