package setup

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// choice is one option in a selector.
type choice struct {
	id    string
	label string
	desc  string
}

// selector is a vertical single-choice list driven by the arrow keys. It keeps
// the wizard's look consistent across the provider, brain, and evidence pickers
// without pulling in the heavier bubbles/list component.
type selector struct {
	choices []choice
	cursor  int
}

func newSelector(choices ...choice) selector { return selector{choices: choices} }

// move advances the cursor by d, wrapping around the ends.
func (s *selector) move(d int) {
	n := len(s.choices)
	if n == 0 {
		return
	}
	s.cursor = (s.cursor + d + n) % n
}

func (s selector) selected() choice {
	if s.cursor < 0 || s.cursor >= len(s.choices) {
		return choice{}
	}
	return s.choices[s.cursor]
}

// fieldSpec describes one text field in a form.
type fieldSpec struct {
	key         string
	label       string
	placeholder string
	value       string // prefilled default
	secret      bool
}

// form is a small vertical stack of text inputs with one focused at a time. Tab
// and the arrow keys move between fields; enter on the last field submits.
type form struct {
	specs  []fieldSpec
	inputs []textinput.Model
	idx    int
}

func newForm(specs ...fieldSpec) form {
	inputs := make([]textinput.Model, len(specs))
	for i, s := range specs {
		ti := textinput.New()
		ti.Placeholder = s.placeholder
		ti.SetValue(s.value)
		ti.Prompt = "› "
		if s.secret {
			ti.EchoMode = textinput.EchoPassword
		}
		if i == 0 {
			ti.Focus()
		}
		inputs[i] = ti
	}
	return form{specs: specs, inputs: inputs}
}

// focus moves the active field to i, updating the focus state of every input.
func (f *form) focus(i int) {
	if len(f.inputs) == 0 {
		return
	}
	f.idx = (i + len(f.inputs)) % len(f.inputs)
	for j := range f.inputs {
		if j == f.idx {
			f.inputs[j].Focus()
		} else {
			f.inputs[j].Blur()
		}
	}
}

// update feeds a key to the focused input and handles field navigation. It
// reports submitted=true when the operator presses enter on the last field.
func (f *form) update(msg tea.Msg) (submitted bool, cmd tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "tab", "down":
			f.focus(f.idx + 1)
			return false, nil
		case "shift+tab", "up":
			f.focus(f.idx - 1)
			return false, nil
		case "enter":
			if f.idx == len(f.inputs)-1 {
				return true, nil
			}
			f.focus(f.idx + 1)
			return false, nil
		}
	}
	f.inputs[f.idx], cmd = f.inputs[f.idx].Update(msg)
	return false, cmd
}

// value returns the trimmed-of-nothing current value for a field key.
func (f form) value(key string) string {
	for i, s := range f.specs {
		if s.key == key {
			return f.inputs[i].Value()
		}
	}
	return ""
}
