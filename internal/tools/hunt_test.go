package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/brain"
	"github.com/brittonhayes/vala/internal/governance"
	"github.com/brittonhayes/vala/internal/policy"
)

// newHuntRC opens a hunt in a fresh Mem-backed brain and returns the run context
// plus the store for assertions.
func newHuntRC(t *testing.T) (*RunContext, *brain.Mem, string) {
	t.Helper()
	mem := brain.NewMem()
	bc := brain.New(mem)
	huntID, err := bc.OpenHunt(context.Background(), brain.Hunt{Question: "q"})
	if err != nil {
		t.Fatalf("OpenHunt: %v", err)
	}
	return NewHuntContext("dev", huntID, bc, policy.Default()), mem, huntID
}

func TestRecordFindingTool(t *testing.T) {
	rc, mem, huntID := newHuntRC(t)
	res := run(t, &RecordFinding{RC: rc}, map[string]any{
		"claim": "DeleteDetector observed", "source": "log_ref", "pointer": "q-1",
	})
	if res.IsError {
		t.Fatalf("record_finding failed: %s", res.Content)
	}
	if !strings.Contains(res.Content, "recorded finding") {
		t.Fatalf("expected a finding id in output: %q", res.Content)
	}
	ev := mem.RowsIn(brain.DBEvidence)
	if len(ev) != 1 || ev[0].Props["hunt"] != huntID {
		t.Fatalf("finding not linked to hunt: %+v", ev)
	}
	// The tool tracks the finding so store_hunt can lint against it.
	if len(rc.Evidence()) != 1 {
		t.Fatalf("expected 1 tracked finding, got %d", len(rc.Evidence()))
	}
}

func TestRecordIntelTool(t *testing.T) {
	rc, mem, huntID := newHuntRC(t)
	res := run(t, &RecordIntel{RC: rc}, map[string]any{
		"kind": "ttp", "value": "attack.t1562.001", "mitre": "attack.t1562.001",
	})
	if res.IsError {
		t.Fatalf("record_intel failed: %s", res.Content)
	}
	rows := mem.RowsIn(brain.DBIntel)
	if len(rows) != 1 {
		t.Fatalf("expected 1 intel row, got %d", len(rows))
	}
	// During a hunt the intel auto-links back to the hunt.
	if got, _ := rows[0].Props["hunts"].([]string); len(got) != 1 || got[0] != huntID {
		t.Fatalf("intel.hunts = %v, want [%s]", rows[0].Props["hunts"], huntID)
	}
}

func TestStoreHuntRejectsUnbackedFinding(t *testing.T) {
	rc, _, _ := newHuntRC(t)
	st := &StoreHunt{RC: rc, Question: "q"}
	res := run(t, st, map[string]any{
		"outcome":  brain.HuntConfirmed,
		"findings": []map[string]any{{"text": "no evidence cited"}},
	})
	if !res.IsError {
		t.Fatal("store_hunt should reject a finding with no evidence")
	}
}

func TestStoreHuntHappyPath(t *testing.T) {
	rc, mem, huntID := newHuntRC(t)
	// Record a finding first so the conclusion can cite it.
	fres := run(t, &RecordFinding{RC: rc}, map[string]any{
		"claim": "fact", "source": "query", "pointer": "q-1",
	})
	fid := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(fres.Content, "—", 2)[0], "recorded finding "))

	st := &StoreHunt{RC: rc, Question: "q"}
	res := run(t, st, map[string]any{
		"outcome":  brain.HuntConfirmed,
		"findings": []map[string]any{{"text": "fact confirmed", "evidence": []string{fid}}},
	})
	if res.IsError {
		t.Fatalf("store_hunt happy path failed: %s", res.Content)
	}
	if got := mem.Rows[huntID].Props["status"]; got != brain.HuntConfirmed {
		t.Fatalf("hunt status = %v, want %q", got, brain.HuntConfirmed)
	}
	if outcome, _ := rc.HuntOutcome(); outcome != brain.HuntConfirmed {
		t.Fatalf("run context outcome = %q, want %q", outcome, brain.HuntConfirmed)
	}
}

func TestLinkArtifactsTool(t *testing.T) {
	rc, mem, huntID := newHuntRC(t)
	detID, err := rc.Brain.RecordDetection(context.Background(), brain.DetectionRef{ID: "rule-1"})
	if err != nil {
		t.Fatalf("RecordDetection: %v", err)
	}
	res := run(t, &LinkArtifacts{RC: rc}, map[string]any{
		"from_id": huntID, "relation": "detections", "to_ids": []string{detID},
	})
	if res.IsError {
		t.Fatalf("link_artifacts failed: %s", res.Content)
	}
	if got, _ := mem.Rows[huntID].Props["detections"].([]string); len(got) != 1 || got[0] != detID {
		t.Fatalf("hunt.detections = %v, want [%s]", mem.Rows[huntID].Props["detections"], detID)
	}
}

// TestHuntToolsAreCaseWrite guards against policy drift: every hunt/intel tool
// must be classified case_write so it is exposed during the hunt phases and
// never treated as a gated action. A tool missing from policy would default to
// action_execute and silently disappear from the hunt.
func TestHuntToolsAreCaseWrite(t *testing.T) {
	pol := policy.Default()
	for _, name := range []string{"record_finding", "store_hunt", "record_intel", "link_artifacts"} {
		if got := pol.ClassOf(name); got != governance.ClassCaseWrite {
			t.Errorf("%s class = %q, want %q", name, got, governance.ClassCaseWrite)
		}
	}
}
