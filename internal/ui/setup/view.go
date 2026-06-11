package setup

import (
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/llm"
	"github.com/charmbracelet/lipgloss"
)

// View renders the active screen centered in a framed card so the wizard reads
// like an unboxing flow rather than a stack of stdout prompts.
func (m model) View() string {
	var body string
	switch m.screen {
	case screenHub:
		body = m.viewHub()
	case screenProviderPick, screenProviderAuth, screenBrainPick, screenEvidencePick:
		body = m.viewSelector()
	case screenProviderOAuth:
		body = m.viewOAuth()
	case screenProviderKey, screenProviderLocal, screenEvidenceForm:
		body = m.viewForm()
	case screenProviderBusy, screenEvidenceBusy:
		body = m.viewBusy()
	case screenBrainNotion:
		body = m.viewNotion()
	case screenEvidenceResult:
		body = m.viewEvidenceResult()
	}
	return m.frame(body)
}

// frame wraps a screen body in the bordered, centered card and footer hint.
func (m model) frame(body string) string {
	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(1, 3).
		Width(64).
		Render(body)

	if m.width == 0 || m.height == 0 {
		return card
	}
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, card)
}

// header is the wizard's title row, shown atop every screen.
func (m model) header(title string) string {
	tag := m.styles.BannerTag.Render("vala")
	return tag + "  " + m.styles.Banner.Render(title)
}

// footerHint renders the contextual key legend for a screen.
func (m model) footerHint(hint string) string {
	line := m.styles.Hint.Render(hint)
	if m.errMsg != "" {
		line = m.styles.Error.Render("✗ "+m.errMsg) + "\n" + line
	}
	return line
}

// viewHub renders the home screen: a selectable checklist of the three steps
// plus "Start hunting". Each row shows live status and is re-enterable, so the
// wizard doubles as a universal setup screen — pick any step to change it.
func (m model) viewHub() string {
	var b strings.Builder
	b.WriteString(m.header("Set up vala") + "\n\n")
	b.WriteString(m.styles.Assistant.Render("Connect what vala needs to hunt. Edit any step anytime.") + "\n\n")
	for i, c := range m.sel.choices {
		done := m.rowDone(c.id)
		cursor := "  "
		label := m.styles.Assistant.Render(c.label)
		if i == m.sel.cursor {
			cursor = m.styles.Prompt.Render("› ")
			label = m.styles.ToolCall.Render(c.label)
		}
		glyph := m.styles.Hint.Render("○")
		if c.id == rowStart {
			glyph = " "
		} else if done {
			glyph = m.styles.Prompt.Render("✓")
		}
		b.WriteString(cursor + glyph + " " + label)
		if c.desc != "" {
			b.WriteString("  " + m.styles.ToolMeta.Render(c.desc))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + m.footerHint("↑/↓ move · enter select · esc start"))
	return b.String()
}

// rowDone reports whether a hub step is satisfied, for its status glyph.
func (m model) rowDone(id string) bool {
	switch id {
	case rowProvider:
		return m.providerDone
	case rowBrain:
		return m.brainDone || m.result.BrainLocal
	case rowEvidence:
		return len(m.evidenceNames()) > 0
	}
	return false
}

func (m model) viewSelector() string {
	var b strings.Builder
	b.WriteString(m.header(m.selectorTitle()) + "\n\n")
	for i, c := range m.sel.choices {
		cursor := "  "
		label := m.styles.Assistant.Render(c.label)
		if i == m.sel.cursor {
			cursor = m.styles.Prompt.Render("› ")
			label = m.styles.ToolCall.Render(c.label)
		}
		b.WriteString(cursor + label)
		if c.desc != "" {
			b.WriteString("  " + m.styles.ToolMeta.Render(c.desc))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n" + m.footerHint("↑/↓ move · enter select · esc skip step"))
	return b.String()
}

func (m model) selectorTitle() string {
	switch m.screen {
	case screenProviderPick:
		return "Step 1 · Connect a model provider"
	case screenProviderAuth:
		return "How do you want to connect " + m.provider.Name + "?"
	case screenBrainPick:
		return "Step 2 · Set up the brain"
	case screenEvidencePick:
		return "Step 3 · Connect evidence sources"
	}
	return ""
}

func (m model) viewForm() string {
	title := "Connect " + m.provider.Name
	if m.screen == screenEvidenceForm {
		title = "Connect " + titleCase(m.evidencePreset)
	}
	var b strings.Builder
	b.WriteString(m.header(title) + "\n\n")
	for i, s := range m.form.specs {
		b.WriteString(m.styles.ToolMeta.Render(s.label) + "\n")
		b.WriteString("  " + m.form.inputs[i].View() + "\n\n")
	}
	b.WriteString(m.footerHint("tab next · enter submit · esc skip"))
	return b.String()
}

func (m model) viewOAuth() string {
	var b strings.Builder
	b.WriteString(m.header("Log in to "+m.provider.Name) + "\n\n")
	b.WriteString(m.styles.Assistant.Render("Your browser is opening. If it didn't, visit:") + "\n")
	b.WriteString(m.styles.ToolMeta.Render(oneLine(m.notice, 56)) + "\n\n")
	b.WriteString(m.styles.ToolMeta.Render("Paste the code shown after you authorize") + "\n")
	b.WriteString("  " + m.form.inputs[0].View() + "\n\n")
	b.WriteString(m.footerHint("enter submit · esc skip"))
	return b.String()
}

func (m model) viewBusy() string {
	return m.header("Working") + "\n\n" + "  " + m.sp.View() + " " + m.styles.SpinnerLabel.Render(m.busyLabel)
}

func (m model) viewNotion() string {
	var b strings.Builder
	b.WriteString(m.header("Notion brain") + "\n\n")
	b.WriteString(m.styles.Assistant.Render("The shared Notion brain is provisioned with the Notion CLI:") + "\n\n")
	b.WriteString("  " + m.styles.ToolCall.Render("ntn login") + m.styles.ToolMeta.Render("   # once, to authenticate") + "\n")
	b.WriteString("  " + m.styles.ToolCall.Render("vala init") + m.styles.ToolMeta.Render("   # provisions the databases") + "\n\n")
	b.WriteString(m.styles.Hint.Render("For now vala will run with no brain; set it up anytime.") + "\n\n")
	b.WriteString(m.footerHint("enter continue"))
	return b.String()
}

func (m model) viewEvidenceResult() string {
	last := m.evidence[len(m.evidence)-1]
	var b strings.Builder
	b.WriteString(m.header("Evidence source") + "\n\n")
	if last.OK() {
		b.WriteString(m.styles.Prompt.Render("✓ connected ") + m.styles.ToolCall.Render(last.Name))
		b.WriteString("  " + m.styles.ToolMeta.Render(fmt.Sprintf("%d tools discovered", last.Tools)) + "\n\n")
		b.WriteString(m.styles.Assistant.Render("vala can hunt in this source now.") + "\n\n")
	} else {
		wrap := lipgloss.NewStyle().Width(56)
		b.WriteString(m.styles.Error.Render("✗ "+last.Name+" did not connect") + "\n")
		b.WriteString(wrap.Foreground(lipgloss.Color("#FF8C8C")).Render(last.Err.Error()) + "\n\n")
		b.WriteString(m.styles.Hint.Render("Saved to .vala.json — fix it and retry.") + "\n\n")
	}
	b.WriteString(m.footerHint("enter back to evidence menu"))
	return b.String()
}

// --- shared helpers ---

// providerHint describes how a provider authenticates, for the picker.
func providerHint(p llm.ProviderInfo) string {
	switch {
	case p.Local:
		return "local server, no key"
	case p.OAuth:
		return "subscription login or API key"
	default:
		return "API key"
	}
}

// titleCase upper-cases the first letter of an ASCII word (preset names).
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// oneLine truncates s to n runes with an ellipsis, collapsing whitespace.
func oneLine(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len([]rune(s)) > n {
		return string([]rune(s)[:n]) + "…"
	}
	return s
}
