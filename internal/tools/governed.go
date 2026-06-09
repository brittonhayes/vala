package tools

import "github.com/brittonhayes/vala/internal/tool"

// GovernedRegistry builds the tool set for a governed incident-response run.
// It includes read-only investigation tools, the discovered MCP evidence
// sources, the case-brain writers, the phase-control tools, and the single
// gated action (slack_notify). Phase exposure and gating are applied on top of
// this by the agent's RunPhase using the policy set — this registry just wires
// every tool to the shared RunContext.
func GovernedRegistry(dir string, rc *RunContext, webhook string, evidence ...tool.Tool) *tool.Registry {
	r := tool.NewRegistry()
	// Evidence sources discovered from configured MCP servers.
	r.Register(evidence...)
	r.Register(
		// Read-only investigation.
		&Read{Dir: dir},
		&LS{Dir: dir},
		&Glob{Dir: dir},
		&Grep{Dir: dir},
		&ReferenceDetection{},
		&ValidateDetection{Dir: dir},
		&TestDetection{Dir: dir},
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
