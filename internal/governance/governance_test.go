package governance

import (
	"encoding/json"
	"testing"
)

func TestActionIDDeterministicAndKeyOrderInsensitive(t *testing.T) {
	a := ActionID("slack_notify", json.RawMessage(`{"message":"hi","channel":"ops"}`))
	b := ActionID("slack_notify", json.RawMessage(`{"channel":"ops","message":"hi"}`))
	if a != b {
		t.Fatalf("expected key-order-insensitive IDs, got %s vs %s", a, b)
	}
	c := ActionID("slack_notify", json.RawMessage(`{"message":"bye","channel":"ops"}`))
	if a == c {
		t.Fatal("different inputs should produce different IDs")
	}
	d := ActionID("other_tool", json.RawMessage(`{"message":"hi","channel":"ops"}`))
	if a == d {
		t.Fatal("different tool should produce different IDs")
	}
}

func TestLedgerLifecycle(t *testing.T) {
	l := NewLedger()
	p := ProposedAction{ID: "act_1", Tool: "slack_notify"}
	l.Propose(p)
	l.Propose(p) // idempotent
	if got := len(l.Proposed()); got != 1 {
		t.Fatalf("re-proposing should not duplicate: got %d", got)
	}
	if l.Satisfied("act_1") {
		t.Fatal("unapproved action must not be satisfied")
	}
	l.Approve("act_1", "operator")
	if !l.Satisfied("act_1") {
		t.Fatal("approved, unexecuted action must be satisfied")
	}
	if !l.MarkExecuted("act_1") {
		t.Fatal("first execution should succeed")
	}
	if l.MarkExecuted("act_1") {
		t.Fatal("second execution must be refused (idempotency)")
	}
	if l.Satisfied("act_1") {
		t.Fatal("executed action must no longer be satisfiable")
	}
	if l.Status("act_1") != StatusExecuted {
		t.Fatalf("status = %q, want executed", l.Status("act_1"))
	}
}

func TestApprovalBindsToSpecificAction(t *testing.T) {
	l := NewLedger()
	l.Propose(ProposedAction{ID: "act_A"})
	l.Propose(ProposedAction{ID: "act_B"})
	l.Approve("act_A", "operator")
	if l.Satisfied("act_B") {
		t.Fatal("approval for A must not satisfy B")
	}
}

func TestToolClassExposure(t *testing.T) {
	if ClassActionExecute.ExposedIn(PhaseEvidence) {
		t.Fatal("actions must not be exposed during evidence")
	}
	if !ClassActionExecute.ExposedIn(PhaseExecute) {
		t.Fatal("actions must be exposed during execute")
	}
	if !ClassRead.ExposedIn(PhaseEvidence) {
		t.Fatal("read tools must be exposed during evidence")
	}
	if ClassRead.ExposedIn(PhaseApproval) {
		t.Fatal("no tools should be exposed during approval")
	}
}
