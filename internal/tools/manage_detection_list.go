package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/brittonhayes/vala/internal/sigma"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed manage_detection_list.md
var manageListDescription string

// listFields are the rule list fields this tool may modify.
var listFields = map[string]bool{
	"references":     true,
	"falsepositives": true,
	"tags":           true,
	"fields":         true,
}

// ManageDetectionList adds or removes a scalar item in a list field
// (references, falsepositives, tags, fields). Not read-only: it edits a file.
type ManageDetectionList struct{ Dir string }

func (m *ManageDetectionList) Name() string        { return "manage_detection_list" }
func (m *ManageDetectionList) Description() string { return manageListDescription }
func (m *ManageDetectionList) ReadOnly() bool      { return false }

func (m *ManageDetectionList) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"path":   map[string]any{"type": "string", "description": "The rule file to edit."},
			"field":  map[string]any{"type": "string", "enum": []string{"references", "falsepositives", "tags", "fields"}, "description": "Which list field to modify."},
			"add":    map[string]any{"type": "string", "description": "Value to append to the list."},
			"remove": map[string]any{"type": "string", "description": "Value to remove from the list."},
		},
		Required: []string{"path", "field"},
	}
}

type manageListInput struct {
	Path   string `json:"path"`
	Field  string `json:"field"`
	Add    string `json:"add"`
	Remove string `json:"remove"`
}

func (m *ManageDetectionList) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in manageListInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if !listFields[in.Field] {
		return tool.Errorf("field must be one of references, falsepositives, tags, fields"), nil
	}
	if in.Add == "" && in.Remove == "" {
		return tool.Errorf("provide 'add' and/or 'remove' a value"), nil
	}

	var summary string
	mutate := func(ed *sigma.Editor) error {
		if in.Remove != "" {
			removed, err := ed.RemoveListItem(in.Field, in.Remove)
			if err != nil {
				return err
			}
			if !removed {
				return fmt.Errorf("%q not found in %s", in.Remove, in.Field)
			}
			summary = fmt.Sprintf("removed %q from %s", in.Remove, in.Field)
		}
		if in.Add != "" {
			if err := ed.AppendListItem(in.Field, in.Add); err != nil {
				return err
			}
			summary = fmt.Sprintf("added %q to %s", in.Add, in.Field)
		}
		return nil
	}
	return editDetection(m.Dir, in.Path, summary, mutate)
}
