package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"strconv"
	"strings"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed edit.md
var editDescription string

// Edit performs an exact string replacement in a file. Not read-only.
type Edit struct{ Dir string }

func (e *Edit) Name() string        { return "edit" }
func (e *Edit) Description() string { return editDescription }
func (e *Edit) ReadOnly() bool      { return false }

func (e *Edit) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"path":        map[string]any{"type": "string", "description": "File to edit."},
			"old_string":  map[string]any{"type": "string", "description": "Exact text to find."},
			"new_string":  map[string]any{"type": "string", "description": "Replacement text."},
			"replace_all": map[string]any{"type": "boolean", "description": "Replace every occurrence."},
		},
		Required: []string{"path", "old_string", "new_string"},
	}
}

type editInput struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all"`
}

func (e *Edit) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in editInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	full := resolve(e.Dir, in.Path)
	data, err := os.ReadFile(full)
	if err != nil {
		return tool.Errorf("cannot read %s: %v", in.Path, err), nil
	}
	content := string(data)

	count := strings.Count(content, in.OldString)
	switch {
	case in.OldString == "":
		return tool.Errorf("old_string is required"), nil
	case count == 0:
		return tool.Errorf("old_string not found in %s", in.Path), nil
	case count > 1 && !in.ReplaceAll:
		return tool.Errorf("old_string found %d times in %s; add context to make it unique or set replace_all", count, in.Path), nil
	}

	var updated string
	if in.ReplaceAll {
		updated = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		updated = strings.Replace(content, in.OldString, in.NewString, 1)
	}
	if err := os.WriteFile(full, []byte(updated), 0o644); err != nil {
		return tool.Errorf("cannot write %s: %v", in.Path, err), nil
	}
	return tool.Text("edited " + in.Path + " (" + plural(count, in.ReplaceAll) + ")"), nil
}

func plural(count int, all bool) string {
	if all && count > 1 {
		return "replaced " + strconv.Itoa(count) + " occurrences"
	}
	return "replaced 1 occurrence"
}
