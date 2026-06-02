package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/brittonhayes/vala/internal/sigma"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed manage_detection_tests.md
var manageTestsDescription string

// ManageDetectionTests adds or removes an inline `tests:` case. Not read-only:
// it edits a file.
type ManageDetectionTests struct{ Dir string }

func (m *ManageDetectionTests) Name() string        { return "manage_detection_tests" }
func (m *ManageDetectionTests) Description() string { return manageTestsDescription }
func (m *ManageDetectionTests) ReadOnly() bool      { return false }

func (m *ManageDetectionTests) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"path":   map[string]any{"type": "string", "description": "The rule file to edit."},
			"name":   map[string]any{"type": "string", "description": "Test case name (used to add or to remove)."},
			"event":  map[string]any{"type": "object", "description": "Sample event for the case. Keys may be dotted, e.g. \"userIdentity.type\"."},
			"match":  map[string]any{"type": "boolean", "description": "Expected outcome: true if the rule should fire on this event."},
			"remove": map[string]any{"type": "boolean", "description": "Remove the case named 'name' instead of adding it."},
		},
		Required: []string{"path", "name"},
	}
}

type manageTestsInput struct {
	Path   string         `json:"path"`
	Name   string         `json:"name"`
	Event  map[string]any `json:"event"`
	Match  bool           `json:"match"`
	Remove bool           `json:"remove"`
}

func (m *ManageDetectionTests) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in manageTestsInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Name == "" {
		return tool.Errorf("provide the test case 'name'"), nil
	}

	var summary string
	mutate := func(ed *sigma.Editor) error {
		if in.Remove {
			removed, err := ed.RemoveMapItem("tests", "name", in.Name)
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("no test case named %q to remove", in.Name)
			}
			summary = fmt.Sprintf("removed test %q", in.Name)
			return nil
		}
		if len(in.Event) == 0 {
			return fmt.Errorf("provide an 'event' for the test case (or set remove=true)")
		}
		// Replace an existing case of the same name so add is idempotent.
		_, _ = ed.RemoveMapItem("tests", "name", in.Name)
		// Encoding a struct preserves field order, so cases read consistently
		// as name → event → match.
		if err := ed.AppendListItem("tests", testCase{Name: in.Name, Event: in.Event, Match: in.Match}); err != nil {
			return err
		}
		summary = fmt.Sprintf("added test %q (match=%v)", in.Name, in.Match)
		return nil
	}
	return editDetection(m.Dir, in.Path, summary, mutate)
}

// testCase marshals to an ordered YAML mapping (name, event, match).
type testCase struct {
	Name  string         `yaml:"name"`
	Event map[string]any `yaml:"event"`
	Match bool           `yaml:"match"`
}
