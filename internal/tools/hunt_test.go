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
	rc := NewRunContext("dev", "", bc, governance.NewLedger(), policy.Default())
	rc.SetHunt(huntID, "q")
	return rc, mem, huntID
}

func TestQueueHuntTool(t *testing.T) {
	mem := brain.NewMem()
	bc := brain.New(mem)
	rc := NewRunContext("dev", "", bc, governance.NewLedger(), policy.Default())
	res := run(t, &QueueHunt{RC: rc}, map[string]any{
		"trigger":     "fresh CVE",
		"hypothesis":  "the CVE is being exploited in our env",
		"behavior":    "exploit attempt against the vulnerable service",
		"data_source": "cloudtrail",
		"priority":    "high",
	})
	if res.IsError {
		t.Fatalf("queue_hunt failed: %s", res.Content)
	}
	rows := mem.RowsIn(brain.DBBacklog)
	if len(rows) != 1 {
		t.Fatalf("expected 1 backlog row, got %d", len(rows))
	}
	if got := rows[0].Props["status"]; got != brain.BacklogQueued {
		t.Fatalf("backlog status = %v, want %q", got, brain.BacklogQueued)
	}
}

// TestOpenHuntConsumesBacklog asserts open_hunt retires the backlog item it was
// opened from and links it to the new hunt.
func TestOpenHuntConsumesBacklog(t *testing.T) {
	mem := brain.NewMem()
	bc := brain.New(mem)
	rc := NewRunContext("dev", "", bc, governance.NewLedger(), policy.Default())
	backlogID, err := bc.QueueHunt(context.Background(), brain.BacklogItem{Trigger: "tg", Hypothesis: "hp"})
	if err != nil {
		t.Fatalf("QueueHunt: %v", err)
	}
	res := run(t, &OpenHunt{RC: rc}, map[string]any{
		"question": "is hp true?", "behavior": "b", "data_source": "cloudtrail", "backlog_id": backlogID,
	})
	if res.IsError {
		t.Fatalf("open_hunt failed: %s", res.Content)
	}
	if got := mem.Rows[backlogID].Props["status"]; got != brain.BacklogOpened {
		t.Fatalf("backlog status = %v, want %q", got, brain.BacklogOpened)
	}
	hunts := mem.RowsIn(brain.DBHunts)
	if len(hunts) != 1 || hunts[0].Props["behavior"] != "b" {
		t.Fatalf("hunt not opened with ABLE behavior: %+v", hunts)
	}
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
	st := &StoreHunt{RC: rc}
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

	st := &StoreHunt{RC: rc}
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

// TestRecallTool exercises the read-only recall tool against a brain that
// already holds a prior hunt and a piece of intel.
func TestRecallTool(t *testing.T) {
	mem := brain.NewMem()
	bc := brain.New(mem)
	ctx := context.Background()
	if _, err := bc.OpenHunt(ctx, brain.Hunt{Question: "did anyone disable GuardDuty?", Behavior: "DeleteDetector"}); err != nil {
		t.Fatalf("OpenHunt: %v", err)
	}
	if _, err := bc.RecordIntel(ctx, brain.Intel{Kind: brain.IntelTTP, Value: "attack.t1562.001"}); err != nil {
		t.Fatalf("RecordIntel: %v", err)
	}
	rc := NewRunContext("dev", "", bc, governance.NewLedger(), policy.Default())
	rec := &Recall{RC: rc}

	// A scoped query surfaces the matching hunt and nothing else.
	res := run(t, rec, map[string]any{"query": "DeleteDetector", "scope": "hunts"})
	if res.IsError {
		t.Fatalf("recall failed: %s", res.Content)
	}
	if !strings.Contains(res.Content, "DeleteDetector") || !strings.Contains(res.Content, "## hunts") {
		t.Fatalf("recall missing the hunt: %q", res.Content)
	}
	if strings.Contains(res.Content, "## intel") {
		t.Fatalf("scope=hunts should not return intel: %q", res.Content)
	}

	// scope=all spans databases.
	all := run(t, rec, map[string]any{"query": "attack.t1562.001"})
	if all.IsError || !strings.Contains(all.Content, "## intel") {
		t.Fatalf("recall all-scope missing intel: %q", all.Content)
	}

	// A miss is a clear "new ground" message, not an error.
	miss := run(t, rec, map[string]any{"query": "nonexistent-behavior"})
	if miss.IsError || !strings.Contains(miss.Content, "new ground") {
		t.Fatalf("expected a new-ground message, got: %q", miss.Content)
	}
}

// TestRecallIsReadClass guards against policy drift: recall must be classified
// read so it stays available during investigation and never requires approval.
// Misclassification would default it to action_execute and hide it.
func TestRecallIsReadClass(t *testing.T) {
	if got := policy.Default().ClassOf("recall"); got != governance.ClassRead {
		t.Errorf("recall class = %q, want %q", got, governance.ClassRead)
	}
}

// TestHuntToolsAreCaseWrite guards against policy drift: every hunt/intel tool
// must be classified case_write so it is exposed during the hunt phases and
// never treated as a gated action. A tool missing from policy would default to
// action_execute and silently disappear from the hunt.
func TestHuntToolsAreCaseWrite(t *testing.T) {
	pol := policy.Default()
	for _, name := range []string{"queue_hunt", "open_hunt", "record_finding", "store_hunt", "record_intel", "link_artifacts"} {
		if got := pol.ClassOf(name); got != governance.ClassCaseWrite {
			t.Errorf("%s class = %q, want %q", name, got, governance.ClassCaseWrite)
		}
	}
}
