// Package governance holds the value types for vala's phase-separated
// detection & response loop: phases, tool classes, the per-call decision
// request/response, proposed actions, and the approval ledger.
//
// It is deliberately a leaf package with no dependencies on agent, permission,
// or policy, so those packages can import it without creating a cycle. The
// phase machine that drives an alert through these phases lives in
// internal/respond; the runtime enforcement that consults these types lives in
// internal/permission and internal/agent.
package governance

// Phase is one stage of the governed loop. The agent is given a different,
// shrinking set of tools in each phase; write/destructive actions are only
// reachable in PhaseExecute, and only with an approval on record.
type Phase string

const (
	// PhasePlan: the model drafts an investigation plan. No tools (or read-only).
	PhasePlan Phase = "plan"
	// PhaseEvidence: read-only gathering plus record_evidence. No actions.
	PhaseEvidence Phase = "evidence"
	// PhasePropose: the model emits explicit action proposals. No execution.
	PhasePropose Phase = "propose"
	// PhaseApproval: human/policy decision. No model tool calls happen here.
	PhaseApproval Phase = "approval"
	// PhaseExecute: only approved action_execute tools may run.
	PhaseExecute Phase = "execute"
	// PhaseReport: write the narrative case page. No destructive tools.
	PhaseReport Phase = "report"
)

// Ordered returns the canonical phase sequence.
func Ordered() []Phase {
	return []Phase{PhasePlan, PhaseEvidence, PhasePropose, PhaseApproval, PhaseExecute, PhaseReport}
}

// ToolClass categorizes a tool for phase exposure and gating. Read-only-ness
// alone is insufficient: case-writing tools (record_evidence, write_case_page)
// are non-read-only yet must be available during investigation, while an
// ntn-backed ad-hoc write is an action. Unknown tools must default to the most
// restricted class so misclassification fails closed.
type ToolClass string

const (
	// ClassRead: observes state only (read, grep, glob, log_search, …).
	ClassRead ToolClass = "read"
	// ClassCaseWrite: writes case-brain artifacts (evidence rows, case page).
	ClassCaseWrite ToolClass = "case_write"
	// ClassControl: drives the phase machine (propose_action, submit_for_approval).
	ClassControl ToolClass = "control"
	// ClassActionExecute: a real side-effecting action (slack_notify, bash, …).
	ClassActionExecute ToolClass = "action_execute"
)

// ExposedIn reports whether a tool of this class may even be shown to the model
// in the given phase. This is the primary, structural defense against scope
// creep: a tool the model never sees cannot be called.
func (c ToolClass) ExposedIn(p Phase) bool {
	switch p {
	case PhasePlan:
		return c == ClassControl
	case PhaseEvidence:
		return c == ClassRead || c == ClassCaseWrite || c == ClassControl
	case PhasePropose:
		return c == ClassControl
	case PhaseApproval:
		return false // no model tool calls in the approval phase
	case PhaseExecute:
		return c == ClassActionExecute
	case PhaseReport:
		return c == ClassRead || c == ClassCaseWrite
	default:
		return false
	}
}
