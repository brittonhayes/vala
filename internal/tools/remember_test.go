package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/brain"
)

// TestRememberWritesSharedMemory covers the multiplayer write: a remembered fact
// lands in the brain stamped with the operator, where it can be recalled — by
// this session or a teammate's pointed at the same brain.
func TestRememberWritesSharedMemory(t *testing.T) {
	mem := brain.NewMem()
	rc := NewRunContext(brain.New(mem))
	rc.Author = "alice"
	rc.SetHunt("hunts_0001", "did anyone disable GuardDuty?")

	res := run(t, &Remember{RC: rc}, map[string]any{"fact": "auth logs live in Okta"})
	if res.IsError {
		t.Fatalf("remember errored: %q", res.Content)
	}
	if !strings.Contains(res.Content, "alice") {
		t.Fatalf("result should name the author: %q", res.Content)
	}

	rows := mem.RowsIn(brain.DBMemory)
	if len(rows) != 1 {
		t.Fatalf("want 1 memory row, got %d", len(rows))
	}
	if got := rows[0].Props["fact"]; got != "auth logs live in Okta" {
		t.Fatalf("fact not stored: %v", got)
	}
	if got := rows[0].Props["author"]; got != "alice" {
		t.Fatalf("author not stamped: %v", got)
	}

	// The fact is readable back through the typed Memories accessor.
	mems, err := rc.Brain.Memories(context.Background(), "Okta", 5)
	if err != nil {
		t.Fatalf("Memories: %v", err)
	}
	if len(mems) != 1 || mems[0].Author != "alice" {
		t.Fatalf("recall did not return the shared memory: %+v", mems)
	}
}

func TestRememberRejectsEmptyFact(t *testing.T) {
	rc := NewRunContext(brain.New(brain.NewMem()))
	res := run(t, &Remember{RC: rc}, map[string]any{"fact": "  "})
	if !res.IsError {
		t.Fatal("empty fact should be rejected")
	}
}

func TestRememberNeedsBrain(t *testing.T) {
	res := run(t, &Remember{RC: &RunContext{}}, map[string]any{"fact": "x"})
	if !res.IsError {
		t.Fatal("remember without a brain should error")
	}
}
