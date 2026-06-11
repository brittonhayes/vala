package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/brain"
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
	rc := NewRunContext(bc)
	rc.SetHunt(huntID, "q", brain.HuntHypothesis)
	return rc, mem, huntID
}

func TestQueueHuntTool(t *testing.T) {
	mem := brain.NewMem()
	bc := brain.New(mem)
	rc := NewRunContext(bc)
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
	rc := NewRunContext(bc)
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
		"outcome":        brain.HuntConfirmed,
		"detection_tier": brain.TierAutomated,
		"tier_rationale": "x",
		"findings":       []map[string]any{{"text": "no evidence cited"}},
		"next_steps":     []string{"follow up"},
	})
	if !res.IsError {
		t.Fatal("store_hunt should reject a finding with no evidence")
	}
}

func TestValidateDataRecordsPlan(t *testing.T) {
	rc, mem, huntID := newHuntRC(t)
	res := run(t, &ValidateData{RC: rc}, map[string]any{
		"sources": []string{"cloudtrail"}, "time_window": "90d", "validated": true,
	})
	if res.IsError {
		t.Fatalf("validate_data failed: %s", res.Content)
	}
	if !rc.DataPlanValidated() {
		t.Fatal("expected data plan to be marked validated")
	}
	ev := mem.RowsIn(brain.DBEvidence)
	if len(ev) != 1 || ev[0].Props["kind"] != brain.EvidenceDataPlan || ev[0].Props["hunt"] != huntID {
		t.Fatalf("expected one data_plan evidence row linked to hunt, got %v", ev)
	}
}

func TestValidateDataRecordsGapOnFailure(t *testing.T) {
	rc, mem, _ := newHuntRC(t)
	res := run(t, &ValidateData{RC: rc}, map[string]any{
		"sources": []string{"vpcflow"}, "validated": false, "gap": "no flow logs retained",
	})
	if res.IsError {
		t.Fatalf("validate_data failed: %s", res.Content)
	}
	if rc.DataPlanValidated() {
		t.Fatal("a failed check must not mark the data plan validated")
	}
	if gaps := rc.Gaps(); len(gaps) != 1 || gaps[0].Source != brain.EvidenceGap {
		t.Fatalf("expected one recorded visibility gap, got %v", gaps)
	}
	ev := mem.RowsIn(brain.DBEvidence)
	if len(ev) != 1 || ev[0].Props["kind"] != brain.EvidenceGap {
		t.Fatalf("expected one visibility_gap evidence row, got %v", ev)
	}
}

func TestStoreHuntRejectsMissingTier(t *testing.T) {
	rc, _, _ := newHuntRC(t)
	run(t, &ValidateData{RC: rc}, map[string]any{"sources": []string{"cloudtrail"}, "validated": true})
	fres := run(t, &RecordFinding{RC: rc}, map[string]any{"claim": "f", "source": "query", "pointer": "q"})
	fid := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(fres.Content, "—", 2)[0], "recorded finding "))
	// detection_tier is required by the schema; omit it and expect rejection.
	res := run(t, &StoreHunt{RC: rc}, map[string]any{
		"outcome":    brain.HuntConfirmed,
		"findings":   []map[string]any{{"text": "f", "evidence": []string{fid}}},
		"next_steps": []string{"x"},
	})
	if !res.IsError {
		t.Fatal("store_hunt should reject a hunt with no detection-tier decision")
	}
}

func TestStoreHuntRejectsQueryBeforeValidation(t *testing.T) {
	rc, _, _ := newHuntRC(t)
	// Query (record a finding) without validating data first.
	fres := run(t, &RecordFinding{RC: rc}, map[string]any{"claim": "f", "source": "query", "pointer": "q"})
	fid := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(fres.Content, "—", 2)[0], "recorded finding "))
	res := run(t, &StoreHunt{RC: rc}, map[string]any{
		"outcome":        brain.HuntConfirmed,
		"detection_tier": brain.TierAutomated,
		"tier_rationale": "x",
		"findings":       []map[string]any{{"text": "f", "evidence": []string{fid}}},
		"next_steps":     []string{"x"},
	})
	if !res.IsError {
		t.Fatal("store_hunt should reject querying before validating data")
	}
}

func TestStoreHuntHappyPath(t *testing.T) {
	rc, mem, huntID := newHuntRC(t)
	// Validate data before querying (stage 3), or store_hunt rejects the hunt.
	if res := run(t, &ValidateData{RC: rc}, map[string]any{
		"sources": []string{"cloudtrail"}, "validated": true,
	}); res.IsError {
		t.Fatalf("validate_data failed: %s", res.Content)
	}
	// Record a finding so the conclusion can cite it.
	fres := run(t, &RecordFinding{RC: rc}, map[string]any{
		"claim": "fact", "source": "query", "pointer": "q-1",
	})
	fid := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(fres.Content, "—", 2)[0], "recorded finding "))

	st := &StoreHunt{RC: rc}
	res := run(t, st, map[string]any{
		"outcome":        brain.HuntConfirmed,
		"detection_tier": brain.TierAutomated,
		"tier_rationale": "clean signal, low false positives",
		"findings":       []map[string]any{{"text": "fact confirmed", "evidence": []string{fid}}},
		"next_steps":     []string{"watch for variant behavior"},
	})
	if res.IsError {
		t.Fatalf("store_hunt happy path failed: %s", res.Content)
	}
	if got := mem.Rows[huntID].Props["status"]; got != brain.HuntConfirmed {
		t.Fatalf("hunt status = %v, want %q", got, brain.HuntConfirmed)
	}
	if got := mem.Rows[huntID].Props["detection_tier"]; got != brain.TierAutomated {
		t.Fatalf("hunt detection_tier = %v, want %q", got, brain.TierAutomated)
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
	rc := NewRunContext(bc)
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
