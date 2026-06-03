package permission

import (
	"encoding/json"
	"testing"

	"github.com/brittonhayes/vala/internal/governance"
	"github.com/brittonhayes/vala/internal/policy"
)

func gate() *Gate {
	g := New(ModeAllow, nil)
	g.Policy = policy.Default()
	return g
}

func actionReq(phase governance.Phase, env string) governance.Request {
	input := json.RawMessage(`{"message":"hi"}`)
	return governance.Request{
		Tool:     "slack_notify",
		Phase:    phase,
		Class:    governance.ClassActionExecute,
		ActionID: governance.ActionID("slack_notify", input),
		Env:      env,
	}
}

func TestDecideBlocksActionDuringEvidence(t *testing.T) {
	g := gate()
	led := governance.NewLedger()
	d := g.Decide(actionReq(governance.PhaseEvidence, "dev"), led)
	if d.Allow {
		t.Fatal("scope creep: action allowed during evidence phase")
	}
}

func TestDecideAllowsAutoApprovedActionInExecute(t *testing.T) {
	g := gate()
	led := governance.NewLedger()
	// slack_notify is auto-approved in dev, so no ledger approval is needed.
	d := g.Decide(actionReq(governance.PhaseExecute, "dev"), led)
	if !d.Allow {
		t.Fatalf("auto-approved action should run in execute: %s", d.Reason)
	}
}

func TestDecideRequiresApprovalWhenNotAuto(t *testing.T) {
	g := gate()
	led := governance.NewLedger()
	req := actionReq(governance.PhaseExecute, "prod") // not auto-approved in prod
	req.Tool = "github_issue"                         // requires approval by default
	req.ActionID = governance.ActionID("github_issue", json.RawMessage(`{}`))
	req.Class = g.Policy.ClassOf("github_issue")
	if d := g.Decide(req, led); d.Allow {
		t.Fatal("action requiring approval ran without one on record")
	}
	led.Approve(req.ActionID, "operator")
	if d := g.Decide(req, led); !d.Allow {
		t.Fatalf("approved action should run: %s", d.Reason)
	}
}

func TestDecideReadOnlyAlwaysAllowed(t *testing.T) {
	g := gate()
	req := governance.Request{Tool: "log_search", Phase: governance.PhaseEvidence, Class: governance.ClassRead, Env: "dev"}
	if d := g.Decide(req, governance.NewLedger()); !d.Allow {
		t.Fatal("read-only tool should always be allowed")
	}
}

func TestDecideHardEnvDeny(t *testing.T) {
	g := gate()
	req := governance.Request{Tool: "bash", Phase: governance.PhaseExecute, Class: governance.ClassActionExecute, Env: "prod", ActionID: "x"}
	if d := g.Decide(req, governance.NewLedger()); d.Allow {
		t.Fatal("bash must be hard-denied in prod")
	}
}
