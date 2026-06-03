package tools

import "github.com/brittonhayes/vala/internal/tool"

// GovernedRegistry builds the tool set for a governed incident-response run.
// It includes read-only investigation tools, the mock evidence source, the
// case-brain writers, the phase-control tools, and the single gated action
// (slack_notify). Phase exposure and gating are applied on top of this by the
// agent's RunPhase using the policy set — this registry just wires every tool to
// the shared RunContext.
func GovernedRegistry(dir string, rc *RunContext, webhook string) *tool.Registry {
	r := tool.NewRegistry()
	r.Register(
		// Read-only investigation.
		&Read{Dir: dir},
		&LS{Dir: dir},
		&Glob{Dir: dir},
		&Grep{Dir: dir},
		&ReferenceDetection{},
		&ValidateDetection{Dir: dir},
		&TestDetection{Dir: dir},
		// Evidence source (mock-capable).
		&LogSearch{Dir: dir},
		// Case-brain writers + phase control.
		&RecordEvidence{RC: rc},
		&ProposeAction{RC: rc},
		&SubmitForApproval{RC: rc},
		&WriteCasePage{RC: rc},
		// The single gated action.
		&SlackNotify{RC: rc, WebhookURL: webhook},
	)
	return r
}
