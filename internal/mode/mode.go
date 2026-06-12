// Package mode defines vala's selectable specializations. A mode is the
// behavioral profile the harness runs in: a headline + workflow-specific system
// prompt, the subset of tools it exposes, the autonomy it prefers, and the
// skills it bundles. Modes are how vala becomes "Claude Code, but for defensive
// security" — the same agent and toolbox, focused for threat hunting, detection
// engineering, and (later) DART/IR or threat-report review.
//
// A mode does NOT own the LLM provider, the permission gate, or the tool
// registry; it only describes a configuration the agent applies (see
// agent.Agent.SetMode). There is one registry for the whole session — a mode
// filters which of its tools are exposed, it never builds a second one — so MCP
// evidence tools and the permission gate are unaffected by the active mode.
package mode

import "github.com/brittonhayes/vala/internal/tool"

// Mode is one selectable specialization.
type Mode struct {
	// ID is the stable identifier used by config, the --mode flag, VALA_MODE,
	// and the /mode command, e.g. "hunt" or "detect". It matches ^[a-z0-9-]+$.
	ID string
	// Title and Description are the human label and one-line help shown by /mode.
	Title       string
	Description string

	// Intro is the headline paragraph that opens the system prompt — it frames
	// what vala is doing in this mode. It is placed before the shared frame
	// (working directory, tools, operating principles).
	Intro string
	// PromptBody returns the workflow-specific middle of the system prompt, placed
	// after the shared operating principles and before the maturity/skills/standing
	// trailer. It receives the same inputs the shared frame sees.
	PromptBody func(PromptInput) string

	// ToolPolicy decides whether a registered tool is exposed to the model in this
	// mode. A nil policy exposes every tool (the hunt default). MCP evidence tools
	// and — when the mode bundles skills — the "skill" tool are always exposed
	// regardless of this policy; the agent enforces that, so a policy never has to
	// remember them (see agent.Agent.modeFilter).
	ToolPolicy func(tool.Tool) bool

	// DefaultMaturity and DefaultPermission are the mode's preferred autonomy.
	// They apply only when the operator did not set autonomy explicitly (config,
	// env, or flag); explicit always wins. A nil DefaultMaturity or empty
	// DefaultPermission means "inherit the session's resolved value".
	DefaultMaturity   *int
	DefaultPermission string

	// Skills lists the ids of bundled skills active in this mode. They are listed
	// in the prompt by name+description (progressive disclosure) and loadable in
	// full via the "skill" tool. Empty means the mode bundles no skills.
	Skills []string
}

// PromptInput carries the shared inputs the headline frame and a mode body see.
type PromptInput struct {
	Workdir       string
	ToolNames     []string // already filtered to the tools the mode exposes
	MaturityLevel int
}

// Allow returns a tool policy that exposes exactly the named tools. Evidence and
// skill tools are force-exposed by the agent regardless, so an allowlist need
// not name them.
func Allow(names ...string) func(tool.Tool) bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(t tool.Tool) bool { return set[t.Name()] }
}

// Deny returns a tool policy that exposes every tool except the named ones. It
// is the safer default for a focused mode: tools added later (and MCP evidence)
// stay available unless explicitly removed.
func Deny(names ...string) func(tool.Tool) bool {
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return func(t tool.Tool) bool { return !set[t.Name()] }
}
