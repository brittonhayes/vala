package ui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/brittonhayes/vala/internal/auth"
	"github.com/brittonhayes/vala/internal/llm"
	"github.com/brittonhayes/vala/internal/mode"
	tea "github.com/charmbracelet/bubbletea"
)

// slashCommand is one operator command invoked with a leading slash. Handlers
// mutate and return the model like any other Bubble Tea update.
type slashCommand struct {
	name    string // without the slash, e.g. "compact"
	desc    string // one-line help text
	handler func(m chatModel, args string) (tea.Model, tea.Cmd)
}

// commands is the ordered registry rendered by /help and dispatched by submit.
func (m chatModel) commands() []slashCommand {
	return []slashCommand{
		{"help", "list available commands", chatModel.cmdHelp},
		{"connect", "choose or switch the LLM provider; /connect <provider> [key]", chatModel.cmdConnect},
		{"mode", "list specializations; /mode <name> switches the active mode live", chatModel.cmdMode},
		{"clear", "clear the conversation and transcript (keep the banner)", chatModel.cmdClear},
		{"compact", "summarize the conversation to reclaim context; optional focus text", chatModel.cmdCompact},
	}
}

// dispatchSlash parses input (which begins with '/') and runs the matching
// command. The bool reports whether input was handled as a slash command; when
// false, the caller treats input as a normal turn. Any '/'-prefixed input is
// handled here — an unknown command reports an error rather than reaching the
// agent.
func (m chatModel) dispatchSlash(input string) (tea.Model, tea.Cmd, bool) {
	if !strings.HasPrefix(input, "/") {
		return m, nil, false
	}
	name, args := splitCommand(input[1:])
	for _, c := range m.commands() {
		if c.name == name {
			model, cmd := c.handler(m, args)
			return model, cmd, true
		}
	}
	m.append("  " + m.styles.Error.Render("unknown command: /"+name) + "  " + m.styles.Hint.Render("try /help"))
	return m, nil, true
}

// matchCommands fuzzy-filters the command registry against a query typed after
// the leading slash, ranking matches so the most relevant command sorts first.
// Name matches outrank description matches, and exact/prefix hits outrank looser
// subsequence ones. An empty query returns every command in registry order so the
// menu doubles as a full command list the moment "/" is typed.
func matchCommands(cmds []slashCommand, query string) []slashCommand {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return cmds
	}
	type scored struct {
		cmd   slashCommand
		score int
	}
	var matched []scored
	for _, c := range cmds {
		if s, ok := scoreCommand(c, query); ok {
			matched = append(matched, scored{c, s})
		}
	}
	// Stable so commands with equal scores keep their registry order.
	sort.SliceStable(matched, func(i, j int) bool { return matched[i].score > matched[j].score })
	out := make([]slashCommand, len(matched))
	for i, mm := range matched {
		out[i] = mm.cmd
	}
	return out
}

// scoreCommand ranks one command against a lowercased query, reporting whether it
// matches at all. Higher is better: an exact name beats a name prefix beats a name
// subsequence beats a description substring beats a description subsequence.
func scoreCommand(c slashCommand, query string) (int, bool) {
	name := strings.ToLower(c.name)
	desc := strings.ToLower(c.desc)
	switch {
	case name == query:
		return 100, true
	case strings.HasPrefix(name, query):
		return 80, true
	case subsequence(query, name):
		return 60, true
	case strings.Contains(desc, query):
		return 40, true
	case subsequence(query, desc):
		return 20, true
	}
	return 0, false
}

// subsequence reports whether every byte of query appears in target in order
// (not necessarily contiguously) — the classic fuzzy-find test, so typing "cmt"
// still finds "compact".
func subsequence(query, target string) bool {
	qi := 0
	for i := 0; i < len(target) && qi < len(query); i++ {
		if target[i] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}

// splitCommand separates the first whitespace-delimited token (the command name)
// from the remaining arguments.
func splitCommand(s string) (name, args string) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i], strings.TrimSpace(s[i+1:])
	}
	return s, ""
}

func (m chatModel) cmdHelp(_ string) (tea.Model, tea.Cmd) {
	var b strings.Builder
	b.WriteString("  " + m.styles.BannerTag.Render("commands") + "\n")
	for _, c := range m.commands() {
		b.WriteString("  " + m.styles.ToolCall.Render("/"+c.name) + "  " + m.styles.Hint.Render(c.desc) + "\n")
	}
	m.append(strings.TrimRight(b.String(), "\n"))
	return m, nil
}

// cmdConnect chooses or switches the active LLM provider mid-session. With no
// arguments it lists providers and their connection state; with a provider id it
// switches to that provider live, optionally storing an API key (remote) or base
// URL (local) passed inline. The credential is persisted to ~/.config/vala so it
// survives restarts; secrets typed inline are visible in the transcript, so the
// guided `vala connect` is the better path for first-time key entry.
func (m chatModel) cmdConnect(args string) (tea.Model, tea.Cmd) {
	if m.running {
		m.append("  " + m.styles.Error.Render("busy") + "  " + m.styles.Hint.Render("wait for the current turn before switching providers"))
		return m, nil
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		m.append(m.connectList())
		return m, nil
	}

	id := fields[0]
	info, ok := llm.Builtin(id)
	if !ok {
		m.append("  " + m.styles.Error.Render("unknown provider: "+id) + "  " + m.styles.Hint.Render("run /connect to list providers"))
		return m, nil
	}

	// An inline secret stores a credential before switching: an API key for a
	// remote provider, or a base URL for a local server.
	if len(fields) >= 2 {
		if err := storeInlineCredential(info, fields[1]); err != nil {
			m.append("  " + m.styles.Error.Render("could not save credential: "+err.Error()))
			return m, nil
		}
	}

	if m.repl.Connect == nil {
		m.append("  " + m.styles.Error.Render("live connect unavailable in this session"))
		return m, nil
	}

	model := info.DefaultModel
	if store, err := auth.Load(); err == nil {
		if cred, ok := store.Get(id); ok && cred.Model != "" {
			model = cred.Model
		}
	}

	provider, err := m.repl.Connect(id, model)
	if err != nil {
		hint := "run `vala connect` for guided setup"
		if info.APIKeyEnv != "" {
			hint = "add a key: /connect " + id + " <api-key>  ·  or set " + info.APIKeyEnv
		} else if info.Local {
			hint = "point at your server: /connect " + id + " <base-url>"
		}
		m.append("  " + m.styles.Error.Render("not connected: "+err.Error()) + "\n  " + m.styles.Hint.Render(hint))
		return m, nil
	}

	m.repl.Agent.SetProvider(provider)
	m.repl.Model = provider.Provider() + " · " + provider.Model()
	m.append("  " + m.styles.BannerTag.Render("connected") + "  " +
		m.styles.ToolCall.Render(provider.Provider()) + "  " + m.styles.Hint.Render(provider.Model()))
	return m, nil
}

// connectList renders the provider picker shown by a bare /connect.
func (m chatModel) connectList() string {
	store, _ := auth.Load()
	var b strings.Builder
	b.WriteString("  " + m.styles.BannerTag.Render("providers") + "\n")
	for _, p := range llm.Providers() {
		mark := " "
		if store != nil {
			if _, ok := store.Get(p.ID); ok {
				mark = "✓"
			}
		}
		if mark == " " && p.APIKeyEnv != "" && os.Getenv(p.APIKeyEnv) != "" {
			mark = "✓"
		}
		hint := "API key"
		switch {
		case p.Local:
			hint = "local server, no key"
		case p.OAuth:
			hint = "subscription login (`vala connect`) or API key"
		case p.APIKeyEnv != "":
			hint = "API key or " + p.APIKeyEnv
		}
		b.WriteString(fmt.Sprintf("  %s %s  %s\n",
			m.styles.ToolGlyph.Render(mark), m.styles.ToolCall.Render(p.ID), m.styles.Hint.Render(hint)))
	}
	b.WriteString("  " + m.styles.Hint.Render("switch: /connect <provider>   ·   add a key: /connect <provider> <api-key>") + "\n")
	b.WriteString("  " + m.styles.Hint.Render("guided setup with masked key entry: run `vala connect` in your shell"))
	return strings.TrimRight(b.String(), "\n")
}

// storeInlineCredential persists a secret passed to /connect: a base URL for a
// local provider, otherwise an API key. It preserves any existing model choice.
func storeInlineCredential(info llm.ProviderInfo, secret string) error {
	store, err := auth.Load()
	if err != nil {
		return err
	}
	cred := auth.Credential{Type: "api"}
	if existing, ok := store.Get(info.ID); ok {
		cred = existing
	}
	if info.Local {
		cred.BaseURL = secret
	} else {
		// An inline secret is an API key, so drop any prior OAuth tokens: the
		// operator is switching this provider back to key auth.
		cred.Type = "api"
		cred.Key = secret
		cred.Access, cred.Refresh, cred.Expiry = "", "", 0
	}
	return store.Set(info.ID, cred)
}

// cmdMode lists the available specializations or switches the active one live.
// With no arguments it prints every mode and marks the active one; with a mode
// id it swaps the agent's mode in place — recomputing the system prompt and the
// exposed tool set — without losing the conversation, mirroring /connect. The
// switch is blocked mid-turn so the running loop's tool set stays consistent.
func (m chatModel) cmdMode(args string) (tea.Model, tea.Cmd) {
	if m.repl.Agent == nil {
		m.append("  " + m.styles.Error.Render("no agent in this session"))
		return m, nil
	}
	name := strings.TrimSpace(args)
	if name == "" {
		m.append(m.modeList())
		return m, nil
	}
	if m.running {
		m.append("  " + m.styles.Error.Render("busy") + "  " + m.styles.Hint.Render("wait for the current turn before switching modes"))
		return m, nil
	}
	mm, ok := mode.Get(name)
	if !ok {
		m.append("  " + m.styles.Error.Render("unknown mode: "+name) + "  " + m.styles.Hint.Render("valid: "+mode.IDs()))
		return m, nil
	}
	m.repl.Agent.SetMode(mm)
	note := mm.Title
	if len(mm.Skills) > 0 {
		note += "  ·  skills: " + strings.Join(mm.Skills, ", ")
	}
	m.append("  " + m.styles.BannerTag.Render("mode") + "  " +
		m.styles.ToolCall.Render(mm.ID) + "  " + m.styles.Hint.Render(note))
	return m, nil
}

// modeList renders the mode picker shown by a bare /mode, marking the active one.
func (m chatModel) modeList() string {
	active := m.repl.Agent.Mode().ID
	var b strings.Builder
	b.WriteString("  " + m.styles.BannerTag.Render("modes") + "\n")
	for _, md := range mode.All() {
		mark := " "
		if md.ID == active {
			mark = "✓"
		}
		b.WriteString(fmt.Sprintf("  %s %s  %s\n",
			m.styles.ToolGlyph.Render(mark), m.styles.ToolCall.Render(md.ID), m.styles.Hint.Render(md.Description)))
	}
	b.WriteString("  " + m.styles.Hint.Render("switch: /mode <name>"))
	return strings.TrimRight(b.String(), "\n")
}

func (m chatModel) cmdClear(_ string) (tea.Model, tea.Cmd) {
	if m.running {
		m.append("  " + m.styles.Error.Render("busy") + "  " + m.styles.Hint.Render("wait for the current turn before clearing"))
		return m, nil
	}
	m.history = nil
	m.blocks = []string{m.banner()} // keep only the banner
	m.lastInputTokens = 0
	m.refreshViewport()
	m.append("  " + m.styles.Hint.Render("context cleared"))
	return m, nil
}

func (m chatModel) cmdCompact(args string) (tea.Model, tea.Cmd) {
	if m.running {
		m.append("  " + m.styles.Error.Render("busy") + "  " + m.styles.Hint.Render("wait for the current turn before compacting"))
		return m, nil
	}
	if len(m.history) == 0 {
		m.append("  " + m.styles.Hint.Render("nothing to compact yet"))
		return m, nil
	}
	return m.startCompaction(args, false)
}
