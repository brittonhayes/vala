// Package permission gates state-changing tool calls. Detection & response
// work routinely touches production systems (Notion, AWS, shells), so by
// default the agent must get explicit approval before any non-read-only tool
// runs. The mode and per-tool allowlist let an operator loosen this when they
// trust a session.
package permission

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

// NextMode returns the mode following m in the ask → allow → deny → ask cycle.
// It backs the interactive shift+tab toggle so an operator can loosen or tighten
// approval without restarting the session.
func NextMode(m Mode) Mode {
	switch m {
	case ModeAsk:
		return ModeAllow
	case ModeAllow:
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

// approveByMode applies the mode/allowlist/prompter decision behind Allow.
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

// CycleMode advances the gate to the next permission mode and returns it.
func (g *Gate) CycleMode() Mode {
	g.Mode = NextMode(g.Mode)
	return g.Mode
}

// AllowTool adds a tool to the allowlist for the remainder of the session,
// e.g. after an operator answers "always allow".
func (g *Gate) AllowTool(name string) {
	if g.allowlist == nil {
		g.allowlist = map[string]bool{}
	}
	g.allowlist[name] = true
}
