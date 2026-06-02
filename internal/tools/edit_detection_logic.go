package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/brittonhayes/vala/internal/sigma"
	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed edit_detection_logic.md
var editLogicDescription string

// EditDetectionLogic manages the detection block: setting/removing a named
// search identifier and setting the condition. Not read-only: it edits a file.
type EditDetectionLogic struct{ Dir string }

func (e *EditDetectionLogic) Name() string        { return "edit_detection_logic" }
func (e *EditDetectionLogic) Description() string { return editLogicDescription }
func (e *EditDetectionLogic) ReadOnly() bool      { return false }

func (e *EditDetectionLogic) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"path":      map[string]any{"type": "string", "description": "The rule file to edit."},
			"selection": map[string]any{"type": "string", "description": "Name of the search identifier to set or remove (e.g. selection, filter_admins)."},
			"fields":    map[string]any{"type": "object", "description": "Field map for the selection. Values may be a string, number, or list (OR). Use field|modifier keys, e.g. \"CommandLine|contains\"."},
			"remove":    map[string]any{"type": "boolean", "description": "With selection, remove that identifier instead of setting it."},
			"condition": map[string]any{"type": "string", "description": "The condition expression, e.g. \"selection and not filter_admins\"."},
		},
		Required: []string{"path"},
	}
}

type editLogicInput struct {
	Path      string         `json:"path"`
	Selection string         `json:"selection"`
	Fields    map[string]any `json:"fields"`
	Remove    bool           `json:"remove"`
	Condition string         `json:"condition"`
}

func (e *EditDetectionLogic) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in editLogicInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Selection == "" && in.Condition == "" {
		return tool.Errorf("provide a 'selection' (to set/remove) and/or a 'condition'"), nil
	}

	var changes []string
	mutate := func(ed *sigma.Editor) error {
		det, err := ed.EnsureMapping("detection")
		if err != nil {
			return err
		}
		if in.Selection != "" {
			if in.Remove {
				if !sigma.DeleteInMapping(det, in.Selection) {
					return fmt.Errorf("no search identifier named %q to remove", in.Selection)
				}
				changes = append(changes, "removed "+in.Selection)
			} else {
				if len(in.Fields) == 0 {
					return fmt.Errorf("provide 'fields' for selection %q (or set remove=true)", in.Selection)
				}
				if err := sigma.SetInMapping(det, in.Selection, in.Fields); err != nil {
					return err
				}
				changes = append(changes, fmt.Sprintf("set %s (%d field(s))", in.Selection, len(in.Fields)))
			}
		}
		if in.Condition != "" {
			sigma.SetScalarInMapping(det, "condition", in.Condition)
			changes = append(changes, "condition="+in.Condition)
		}
		return nil
	}
	return editDetection(e.Dir, in.Path, "detection logic: "+strings.Join(changes, "; "), mutate)
}
