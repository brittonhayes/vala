package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/skills"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed skill.md
var skillDescription string

// Skill returns the full body of a bundled skill on demand. It is the load step
// of progressive disclosure: the active skills' names and descriptions live in
// the system prompt, and this tool fetches the full instructions only when the
// agent decides it needs them. Read-only: it returns static guidance.
type Skill struct{ Set *skills.Set }

func (s *Skill) Name() string        { return "skill" }
func (s *Skill) Description() string { return skillDescription }
func (s *Skill) ReadOnly() bool      { return true }

func (s *Skill) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"name": map[string]any{"type": "string", "description": "Return the full instructions for the skill with this name."},
			"list": map[string]any{"type": "boolean", "description": "List every available skill (name and description)."},
		},
	}
}

type skillInput struct {
	Name string `json:"name"`
	List bool   `json:"list"`
}

func (s *Skill) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in skillInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if s.Set == nil {
		return tool.Text("no skills are available."), nil
	}

	if in.Name != "" && !in.List {
		sk, ok := s.Set.Get(in.Name)
		if !ok {
			return tool.Errorf("no skill named %q (use {\"list\": true} to see available skills)", in.Name), nil
		}
		return tool.Text(sk.Body), nil
	}

	all := s.Set.All()
	if len(all) == 0 {
		return tool.Text("no skills are available."), nil
	}
	var b strings.Builder
	b.WriteString("Available skills — load one in full with {\"name\": \"<name>\"}.\n\n")
	for _, sk := range all {
		fmt.Fprintf(&b, "• %s — %s\n", sk.Name, sk.Description)
	}
	return tool.Text(strings.TrimRight(b.String(), "\n")), nil
}
