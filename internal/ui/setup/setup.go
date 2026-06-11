// Package setup renders vala's first-run onboarding: a single curated,
// full-screen Bubble Tea flow that connects a model provider, sets up the brain,
// and connects the security-tool MCP evidence sources the agent hunts in. It
// runs before the REPL whenever the session detects an unconfigured surface, so
// the operator is guided from "nothing to hunt in" to a working tool without
// hunting for separate commands.
package setup

import (
	"context"
	"time"

	"github.com/brittonhayes/vala/internal/config"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/mcp"
	"github.com/brittonhayes/vala/internal/ui"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// screen identifies the active panel in the wizard's state machine. The hub is
// the home screen: every step is reachable from it and returns to it, so the
// wizard works as a universal setup surface — re-enter any step to change it.
type screen int

const (
	screenHub screen = iota
	screenProviderPick
	screenProviderAuth // OAuth-vs-key choice for an OAuth-capable provider
	screenProviderOAuth
	screenProviderKey
	screenProviderLocal
	screenProviderBusy
	screenBrainPick
	screenBrainNotion
	screenEvidencePick
	screenEvidenceForm
	screenEvidenceBusy
	screenEvidenceResult
)

// Options configures a wizard run. The OK flags come from the caller's
// setup-state detection; a step that is already satisfied renders as ✓ and is
// skippable, so re-running the wizard to add one missing piece is painless.
type Options struct {
	Cwd        string
	ProviderOK bool
	BrainOK    bool
	Model      string   // active provider·model label, for the ✓ provider row
	Brain      string   // current brain summary (e.g. "on-disk" / "Notion" / "none")
	Evidence   []string // names of evidence sources already configured in .vala.json
	Force      bool     // unused hook for future "show all" behavior
}

// Result reports what the operator decided so the caller can finish the work
// the TUI deliberately leaves to existing helpers (local-brain provisioning) and
// decide whether to launch the session.
type Result struct {
	// Quit is true when the operator pressed ctrl+c to abort vala entirely.
	Quit bool
	// BrainLocal is true when the operator chose the on-disk brain; the caller
	// provisions it with its existing helper (which also scaffolds VALA.md).
	BrainLocal bool
}

// Run launches the wizard and blocks until the operator finishes or skips it.
func Run(ctx context.Context, opts Options) (Result, error) {
	m := newModel(ctx, opts)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return Result{}, err
	}
	fm, _ := final.(model)
	return fm.result, nil
}

// model is the wizard's Bubble Tea model.
type model struct {
	ctx    context.Context
	styles ui.Styles
	opts   Options
	sp     spinner.Model

	width, height int

	screen screen
	sel    selector
	form   form

	// provider sub-flow state.
	provider      llm.ProviderInfo
	oauthVerifier string

	// evidencePreset is the source kind being configured ("scanner"/"wiz"/"custom").
	evidencePreset string

	// evidence accumulates the sources connected during this run, shown on the
	// checklist and the final summary.
	evidence []mcp.EvidenceStatus
	// pendingServer is the source currently being saved and validated.
	pendingServer config.MCPServer

	// transient banners.
	notice string // neutral status (e.g. "browser opened")
	errMsg string // last error, shown in red until the next action

	// live checklist status, updated as steps complete.
	providerDone bool
	brainDone    bool

	busyLabel string
	result    Result
}

// newModel builds the initial wizard model positioned on the welcome screen.
func newModel(ctx context.Context, opts Options) model {
	sp := spinner.New()
	st := ui.DefaultStyles()
	sp.Spinner = spinner.Spinner{Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}, FPS: time.Second / 10}
	sp.Style = st.Spinner
	m := model{
		ctx:          ctx,
		styles:       st,
		opts:         opts,
		sp:           sp,
		providerDone: opts.ProviderOK,
		brainDone:    opts.BrainOK,
	}
	hub, _ := m.toHub()
	return hub.(model)
}

func (m model) Init() tea.Cmd { return m.sp.Tick }
