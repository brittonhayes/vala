package ui

import (
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/tools"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type choiceMsg struct {
	req   tools.ChoiceRequest
	reply chan tools.ChoiceResponse
}

type choicePrompt struct {
	req      tools.ChoiceRequest
	reply    chan tools.ChoiceResponse
	cursor   int
	selected map[string]bool
	chatting bool
}

func newChoicePrompt(msg choiceMsg) *choicePrompt {
	c := &choicePrompt{
		req:      msg.req,
		reply:    msg.reply,
		selected: map[string]bool{},
	}
	firstDefault := -1
	for i, opt := range msg.req.Options {
		if opt.Default {
			c.selected[opt.ID] = true
			if firstDefault < 0 {
				firstDefault = i
			}
		}
	}
	if msg.req.Mode == tools.ChoiceSingle {
		c.selected = map[string]bool{}
		idx := 0
		if firstDefault >= 0 {
			idx = firstDefault
		}
		c.cursor = idx
		c.selected[msg.req.Options[idx].ID] = true
	} else if firstDefault >= 0 {
		c.cursor = firstDefault
	}
	return c
}

func (c *choicePrompt) maxCursor() int {
	max := len(c.req.Options) - 1
	if c.req.AllowChat {
		max++
	}
	return max
}

func (c *choicePrompt) onChatRow() bool {
	return c.req.AllowChat && c.cursor == len(c.req.Options)
}

func (c *choicePrompt) move(delta int) {
	c.cursor += delta
	if c.cursor < 0 {
		c.cursor = c.maxCursor()
	}
	if c.cursor > c.maxCursor() {
		c.cursor = 0
	}
}

func (c *choicePrompt) toggleCursor() {
	if c.onChatRow() || c.cursor < 0 || c.cursor >= len(c.req.Options) {
		c.chatting = true
		return
	}
	opt := c.req.Options[c.cursor]
	if c.req.Mode == tools.ChoiceSingle {
		c.selected = map[string]bool{opt.ID: true}
		return
	}
	c.selected[opt.ID] = !c.selected[opt.ID]
}

func (c *choicePrompt) selectedIDs() []string {
	out := make([]string, 0, len(c.selected))
	for _, opt := range c.req.Options {
		if c.selected[opt.ID] {
			out = append(out, opt.ID)
		}
	}
	return out
}

func (c *choicePrompt) optionIndexForKey(key string) int {
	key = strings.ToLower(strings.TrimSpace(key))
	if key == "" {
		return -1
	}
	for i, opt := range c.req.Options {
		if strings.ToLower(opt.ID) == key {
			return i
		}
		if fmt.Sprintf("%d", i+1) == key {
			return i
		}
	}
	return -1
}

func (m chatModel) onChoiceKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.choice == nil {
		return m, nil
	}
	if m.choice.chatting {
		switch msg.String() {
		case "enter":
			text := strings.TrimSpace(m.ta.Value())
			if text == "" {
				m.choice.chatting = false
				m.ta.Blur()
				m.relayout()
				return m, nil
			}
			m.answerChoice(tools.ChoiceResponse{Message: text})
			return m, nil
		case "esc":
			if strings.TrimSpace(m.ta.Value()) == "" {
				m.choice.chatting = false
				m.ta.Blur()
			} else {
				m.ta.Reset()
			}
			m.relayout()
			return m, nil
		case "ctrl+c":
			m.answerChoice(tools.ChoiceResponse{Canceled: true})
			return m, nil
		}
		var cmd tea.Cmd
		m.ta, cmd = m.ta.Update(msg)
		m.relayout()
		return m, cmd
	}

	switch msg.String() {
	case "up", "ctrl+p":
		m.choice.move(-1)
		m.relayout()
		return m, nil
	case "down", "ctrl+n":
		m.choice.move(1)
		m.relayout()
		return m, nil
	case " ":
		if m.choice.onChatRow() {
			m.beginChoiceChat()
			return m, nil
		}
		m.choice.toggleCursor()
		m.relayout()
		return m, nil
	case "enter":
		if m.choice.onChatRow() {
			m.beginChoiceChat()
			return m, nil
		}
		if m.choice.req.Mode == tools.ChoiceSingle {
			m.choice.toggleCursor()
		}
		m.answerChoice(tools.ChoiceResponse{Selected: m.choice.selectedIDs()})
		return m, nil
	case "tab":
		if m.choice.req.AllowChat {
			m.beginChoiceChat()
			return m, nil
		}
	case "esc", "ctrl+c":
		m.answerChoice(tools.ChoiceResponse{Canceled: true})
		return m, nil
	}

	if idx := m.choice.optionIndexForKey(msg.String()); idx >= 0 {
		m.choice.cursor = idx
		m.choice.toggleCursor()
		if m.choice.req.Mode == tools.ChoiceSingle {
			m.answerChoice(tools.ChoiceResponse{Selected: m.choice.selectedIDs()})
			return m, nil
		}
		m.relayout()
		return m, nil
	}

	if m.choice.req.AllowChat && len([]rune(msg.String())) == 1 {
		m.beginChoiceChat()
		m.ta.SetValue(msg.String())
		m.ta.CursorEnd()
		m.relayout()
		return m, nil
	}
	return m, nil
}

func (m *chatModel) beginChoiceChat() {
	if m.choice == nil {
		return
	}
	m.choice.chatting = true
	m.ta.Reset()
	m.ta.Placeholder = choicePlaceholder
	m.ta.Focus()
	m.relayout()
}

func (m *chatModel) answerChoice(ans tools.ChoiceResponse) {
	if m.choice == nil {
		return
	}
	m.choice.reply <- ans
	m.choice = nil
	m.ta.Reset()
	m.ta.Placeholder = inputPlaceholder
	m.ta.Focus()
	m.relayout()
}

func (m chatModel) choiceView() string {
	if m.choice == nil {
		return ""
	}
	req := m.choice.req
	width := max(24, m.width-4)
	var lines []string
	for _, line := range wrapChoiceText("? "+req.Question, width) {
		lines = append(lines, uiGutter+m.styles.ChoiceTitle.Render(line))
	}
	for i, opt := range req.Options {
		cursor := " "
		if i == m.choice.cursor {
			cursor = "›"
		}
		mark := "( )"
		if req.Mode == tools.ChoiceMulti {
			mark = "[ ]"
		}
		if m.choice.selected[opt.ID] {
			if req.Mode == tools.ChoiceMulti {
				mark = "[x]"
			} else {
				mark = "(*)"
			}
		}
		gutter := fmt.Sprintf("%s %s ", cursor, mark)
		body := opt.ID
		if opt.Label != "" && opt.Label != opt.ID {
			body += "  " + opt.Label
		}
		if opt.Detail != "" {
			body += "  " + opt.Detail
		}
		bodyWidth := max(1, width-lipgloss.Width(gutter))
		wrapped := wrapChoiceText(body, bodyWidth)
		if i == m.choice.cursor {
			lines = append(lines, renderChoiceRow(m.styles.ChoiceCursor, gutter, wrapped)...)
		} else if m.choice.selected[opt.ID] {
			lines = append(lines, renderChoiceRow(m.styles.ChoiceSelected, gutter, wrapped)...)
		} else {
			lines = append(lines, renderChoiceRow(m.styles.Hint, gutter, wrapped)...)
		}
	}
	if req.AllowChat {
		line := "  tab  Chat instead"
		if m.choice.onChatRow() {
			lines = append(lines, uiGutter+m.styles.ChoiceCursor.Render("› "+line))
		} else {
			lines = append(lines, uiGutter+m.styles.ToolMeta.Render("  "+line))
		}
	}
	return strings.Join(lines, "\n")
}

func (m chatModel) choiceHeight() int {
	if m.choice == nil {
		return 0
	}
	return strings.Count(m.choiceView(), "\n") + 1
}

func (m chatModel) choiceFooter() string {
	if m.choice == nil {
		return ""
	}
	if m.choice.chatting {
		return uiGutter + m.styles.Hint.Render("enter send · esc back · ctrl+c cancel")
	}
	action := "enter select"
	if m.choice.req.Mode == tools.ChoiceMulti {
		action = "space toggle · enter send"
	}
	chat := ""
	if m.choice.req.AllowChat {
		chat = " · tab chat"
	}
	return uiGutter + m.styles.Hint.Render("↑/↓ move · "+action+chat+" · esc cancel")
}

func renderChoiceRow(style lipgloss.Style, gutter string, body []string) []string {
	if len(body) == 0 {
		body = []string{""}
	}
	out := make([]string, 0, len(body))
	indent := strings.Repeat(" ", lipgloss.Width(gutter))
	for i, line := range body {
		prefix := gutter
		if i > 0 {
			prefix = indent
		}
		out = append(out, uiGutter+style.Render(prefix+line))
	}
	return out
}

func wrapChoiceText(text string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, line := range strings.Split(text, "\n") {
		for _, wrapped := range softWrap([]rune(line), width) {
			out = append(out, strings.TrimRight(string(wrapped), " "))
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}
