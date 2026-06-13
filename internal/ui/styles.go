package ui

import "github.com/charmbracelet/lipgloss"

// Styles holds the lipgloss styles used to render the REPL.
type Styles struct {
	Banner       lipgloss.Style
	BannerTag    lipgloss.Style
	Prompt       lipgloss.Style
	Assistant    lipgloss.Style
	ToolCall     lipgloss.Style
	ToolGlyph    lipgloss.Style
	ToolMeta     lipgloss.Style
	Gutter       lipgloss.Style
	Result       lipgloss.Style
	ResultErr    lipgloss.Style
	Error        lipgloss.Style
	Denied       lipgloss.Style
	Hint         lipgloss.Style
	Rule         lipgloss.Style
	Spinner      lipgloss.Style
	SpinnerLabel lipgloss.Style
	InputBox     lipgloss.Style
	InputBoxBusy lipgloss.Style
	Queued       lipgloss.Style
	User         lipgloss.Style

	// Permission styles are deliberately high-contrast so an approval request is
	// impossible to miss against the dim transcript.
	Permission    lipgloss.Style
	PermissionKey lipgloss.Style
	Mode          lipgloss.Style

	// Completion styles render the slash-command autocomplete menu: the
	// highlighted row stands out against the dim, unselected rows.
	CompletionName lipgloss.Style
	CompletionSel  lipgloss.Style
}

// DefaultStyles returns the vala color palette.
func DefaultStyles() Styles {
	return Styles{
		Banner:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7D56F4")),
		BannerTag:    lipgloss.NewStyle().Foreground(lipgloss.Color("#0B0B0F")).Background(lipgloss.Color("#7D56F4")).Bold(true).Padding(0, 1),
		Prompt:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#43BF6D")),
		Assistant:    lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6")),
		ToolCall:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00B4D8")),
		ToolGlyph:    lipgloss.NewStyle().Foreground(lipgloss.Color("#00B4D8")),
		ToolMeta:     lipgloss.NewStyle().Foreground(lipgloss.Color("#7A7A7A")),
		Gutter:       lipgloss.NewStyle().Foreground(lipgloss.Color("#3A3A40")),
		Result:       lipgloss.NewStyle().Foreground(lipgloss.Color("#9A9A9A")),
		ResultErr:    lipgloss.NewStyle().Foreground(lipgloss.Color("#FF8C8C")),
		Error:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FF5C57")),
		Denied:       lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB454")),
		Hint:         lipgloss.NewStyle().Foreground(lipgloss.Color("#6A6A72")),
		Rule:         lipgloss.NewStyle().Foreground(lipgloss.Color("#2A2A30")),
		Spinner:      lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")),
		SpinnerLabel: lipgloss.NewStyle().Foreground(lipgloss.Color("#B9B0E6")),
		InputBox:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#3A3A40")).Padding(0, 1),
		InputBoxBusy: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#7D56F4")).Padding(0, 1),
		Queued:       lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB454")),
		User:         lipgloss.NewStyle().Foreground(lipgloss.Color("#E6E6E6")),

		Permission:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FFFFFF")),
		PermissionKey: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#B197FC")),
		Mode:          lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#B197FC")),

		CompletionName: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#00B4D8")),
		CompletionSel:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#0B0B0F")).Background(lipgloss.Color("#7D56F4")),
	}
}
