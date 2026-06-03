// Package permission gates state-changing tool calls. Detection & response
// work routinely touches production systems (Notion, AWS, shells), so by
// default the agent must get explicit approval before any non-read-only tool
// runs. The mode and per-tool allowlist let an operator loosen this when they
// trust a session.
package permission

import (
	"fmt"

	"github.com/brittonhayes/vala/internal/governance"
	"github.com/brittonhayes/vala/internal/policy"
)

// Mode controls the default disposition for non-read-only tool calls.
type Mode string

const (
	// ModeAsk prompts the operator for each non-read-only, non-allowlisted call.
	ModeAsk Mode = "ask"
	// ModeAllow auto-approves every call (use for trusted, unattended runs).
	ModeAllow Mode = "allow"
	// ModeDeny rejects every non-read-only call (dry-run / investigation only).
	ModeDeny Mode = "deny"
)

// Parse converts a string into a Mode, defaulting to ModeAsk.
func Parse(s string) Mode {
	switch Mode(s) {
	case ModeAllow:
		return ModeAllow
	case ModeDeny:
		return ModeDeny
	default:
		return ModeAsk
	}
}

// Prompter asks the operator to approve a single call. It receives the tool
// name and a short human summary of the call and returns true to allow it.
type Prompter func(tool, summary string) bool

// Gate decides whether a tool call may proceed.
type Gate struct {
	Mode      Mode
	allowlist map[string]bool
	Prompt    Prompter

	// Policy is consulted by Decide for the governed (phase-separated) loop. It
	// is nil for the legacy single-phase Allow path.
	Policy *policy.Set
}

// New builds a Gate from a mode and a list of always-allowed tool names.
func New(mode Mode, allowlist []string) *Gate {
	set := make(map[string]bool, len(allowlist))
	for _, name := range allowlist {
		set[name] = true
	}
	return &Gate{Mode: mode, allowlist: set}
}

// Allow reports whether a call should run. Read-only calls always proceed.
// Otherwise the decision follows the mode, the allowlist, and finally the
// interactive prompter (if one is configured).
func (g *Gate) Allow(tool, summary string, readOnly bool) bool {
	if readOnly {
		return true
	}
	return g.approveByMode(tool, summary)
}

// approveByMode applies the mode/allowlist/prompter decision shared by the
// legacy Allow path and the governed Decide path's final human gate.
func (g *Gate) approveByMode(tool, summary string) bool {
	if g == nil || g.Mode == ModeAllow {
		return true
	}
	if g.Mode == ModeDeny {
		return false
	}
	if g.allowlist[tool] {
		return true
	}
	if g.Prompt != nil {
		return g.Prompt(tool, summary)
	}
	// No way to ask and not explicitly allowed: fail closed.
	return false
}

// Decide is the phase- and ledger-aware verdict used by the governed loop. It
// is the authoritative enforcement point for F2: a write/destructive action can
// only run in PhaseExecute with an approval on record. Checks are ordered and
// fail closed.
func (g *Gate) Decide(req governance.Request, led *governance.Ledger) governance.Decision {
	pol := g.Policy
	if pol == nil {
		pol = policy.Default()
	}

	// 1. Read / control / case-writing tools are not side-effecting actions and
	//    are governed only by phase exposure (enforced upstream) — allow.
	if req.Class != governance.ClassActionExecute {
		return governance.Decision{Allow: true}
	}

	// 2. Hard environment deny.
	if pol.EnvDenied(req.Env, req.Tool) {
		return deny("tool %q is denied in env %q", req.Tool, req.Env)
	}

	// 3. Actions may only execute in the Execute phase (no scope creep).
	if req.Phase != governance.PhaseExecute {
		return deny("scope creep: action %q attempted in phase %q (allowed only in execute)", req.Tool, req.Phase)
	}

	// 4. The action must be approved (or policy auto-approved) and not already
	//    executed. The approval binds to this exact action ID.
	if pol.ApprovalRequired(req.Env, req.Tool) && (led == nil || !led.Satisfied(req.ActionID)) {
		return deny("no approval on record for action %s (%s)", req.ActionID, req.Tool)
	}
	if led != nil && req.ActionID != "" && led.Status(req.ActionID) == governance.StatusExecuted {
		return deny("action %s already executed (idempotency)", req.ActionID)
	}

	// 5. Final human gate (mode / allowlist / prompter).
	if !g.approveByMode(req.Tool, req.Summary) {
		return deny("operator declined %q", req.Tool)
	}
	return governance.Decision{Allow: true}
}

func deny(format string, args ...any) governance.Decision {
	return governance.Decision{Allow: false, Reason: fmt.Sprintf(format, args...)}
}

// AllowTool adds a tool to the allowlist for the remainder of the session,
// e.g. after an operator answers "always allow".
func (g *Gate) AllowTool(name string) {
	if g.allowlist == nil {
		g.allowlist = map[string]bool{}
	}
	g.allowlist[name] = true
}
