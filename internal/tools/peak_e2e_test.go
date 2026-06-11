package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/brittonhayes/vala/internal/brain"
)

// TestPEAKLoopEndToEnd drives the whole loop through the tools against a durable
// file-backed brain — no model, no network — and asserts the brain ends in the
// expected state: a validated data plan, a cited finding, a closed hunt with a
// tier decision, and an updated coverage row.
func TestPEAKLoopEndToEnd(t *testing.T) {
	ctx := context.Background()
	store, err := brain.NewFile(filepath.Join(t.TempDir(), "brain.json"))
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	bc := brain.New(store)
	rc := NewRunContext(bc)
	rc.Author = "tester"

	// 2. Form hypothesis.
	if res := run(t, &OpenHunt{RC: rc}, map[string]any{
		"question": "did anyone disable GuardDuty?", "behavior": "DeleteDetector",
		"data_source": "cloudtrail", "hunt_type": brain.HuntHypothesis,
	}); res.IsError {
		t.Fatalf("open_hunt: %s", res.Content)
	}
	// 3. Plan & validate data.
	if res := run(t, &ValidateData{RC: rc}, map[string]any{
		"sources": []string{"cloudtrail"}, "time_window": "90d", "validated": true,
	}); res.IsError {
		t.Fatalf("validate_data: %s", res.Content)
	}
	// 4. Execute & analyze.
	fres := run(t, &RecordFinding{RC: rc}, map[string]any{
		"claim": "DeleteDetector by svc-x", "source": "query",
		"pointer": "eventName=DeleteDetector", "confidence": "confirmed",
	})
	if fres.IsError {
		t.Fatalf("record_finding: %s", fres.Content)
	}
	fid := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(fres.Content, "—", 2)[0], "recorded finding "))
	// 6. Document & decide.
	if res := run(t, &StoreHunt{RC: rc}, map[string]any{
		"outcome": brain.HuntConfirmed, "detection_tier": brain.TierAutomated,
		"tier_rationale": "DeleteDetector is unambiguous",
		"findings":       []map[string]any{{"text": "GuardDuty was disabled", "evidence": []string{fid}}},
		"next_steps":     []string{"watch for re-enable"},
	}); res.IsError {
		t.Fatalf("store_hunt: %s", res.Content)
	}
	// 8. Feed back.
	if res := run(t, &UpdateCoverage{RC: rc}, map[string]any{
		"technique": "attack.t1562.001", "status": brain.CoverageCovered, "fidelity": "high",
	}); res.IsError {
		t.Fatalf("update_coverage: %s", res.Content)
	}

	// The hunt closed with the tier decision recorded.
	hunts, _ := bc.Recall(ctx, brain.DBHunts, "", 10)
	if len(hunts) != 1 || hunts[0].Props["status"] != brain.HuntConfirmed || hunts[0].Props["detection_tier"] != brain.TierAutomated {
		t.Fatalf("hunt not closed with tier: %+v", hunts)
	}
	// Coverage was upserted for the technique.
	cov, _ := bc.Recall(ctx, brain.DBCoverage, "t1562", 10)
	if len(cov) != 1 || cov[0].Props["status"] != brain.CoverageCovered {
		t.Fatalf("coverage not recorded: %+v", cov)
	}
}
