package tools

import "github.com/brittonhayes/vala/internal/tool"

// HuntRegistry builds the tool set for a `vala hunt` run: read-only
// investigation tools, the mock evidence source, and the hunt/intel brain
// writers. Hunting is read-mostly — it has no proposal/approval/execute action
// path — so no action_execute tools are wired. Phase exposure and gating are
// applied on top by the agent's RunPhase using the policy set; this registry
// just wires every tool to the shared RunContext.
func HuntRegistry(dir, question string, rc *RunContext) *tool.Registry {
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
		// Brain writers: findings, intel, links, and the hunt page.
		&RecordFinding{RC: rc},
		&RecordIntel{RC: rc},
		&LinkArtifacts{RC: rc},
		&StoreHunt{RC: rc, Question: question},
	)
	return r
}
