package policy

import (
	"testing"

	"github.com/brittonhayes/vala/internal/governance"
)

func TestUnknownToolFailsClosed(t *testing.T) {
	p := Default()
	if got := p.ClassOf("totally_new_tool"); got != governance.ClassActionExecute {
		t.Fatalf("unknown tool should default to action_execute, got %q", got)
	}
	// And therefore must not be exposed during investigation.
	if p.ExposeInPhase("totally_new_tool", governance.PhaseEvidence, "dev") {
		t.Fatal("unknown tool must not be exposed during evidence")
	}
}

func TestExposeInPhase(t *testing.T) {
	p := Default()
	if !p.ExposeInPhase("log_search", governance.PhaseEvidence, "dev") {
		t.Fatal("log_search should be exposed in evidence")
	}
	if p.ExposeInPhase("slack_notify", governance.PhaseEvidence, "dev") {
		t.Fatal("slack_notify must not be exposed in evidence")
	}
	if !p.ExposeInPhase("slack_notify", governance.PhaseExecute, "dev") {
		t.Fatal("slack_notify should be exposed in execute")
	}
}

func TestApprovalAndAutoApprove(t *testing.T) {
	p := Default()
	if p.ApprovalRequired("dev", "slack_notify") {
		t.Fatal("slack_notify is auto-approved in dev")
	}
	if !p.AutoApprove("dev", "slack_notify") {
		t.Fatal("slack_notify should auto-approve in dev")
	}
	if !p.ApprovalRequired("dev", "github_issue") {
		t.Fatal("unlisted action should require approval by default")
	}
}

func TestEnvDeny(t *testing.T) {
	p := Default()
	if !p.EnvDenied("prod", "bash") {
		t.Fatal("bash should be denied in prod")
	}
	if p.EnvDenied("dev", "bash") {
		t.Fatal("bash should be allowed in dev")
	}
}

func TestRequiresEvidence(t *testing.T) {
	p := Default()
	if !p.RequiresEvidence("slack_notify") {
		t.Fatal("slack_notify should require evidence")
	}
}
