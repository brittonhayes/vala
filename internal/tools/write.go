package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed write.md
var writeDescription string

// Write creates or overwrites a file. Not read-only; permission-gated.
type Write struct{ Dir string }

func (w *Write) Name() string        { return "write" }
func (w *Write) Description() string { return writeDescription }
func (w *Write) ReadOnly() bool      { return false }

func (w *Write) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"path":    map[string]any{"type": "string", "description": "Destination file path."},
			"content": map[string]any{"type": "string", "description": "Full file contents."},
		},
		Required: []string{"path", "content"},
	}
}

type writeInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (w *Write) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in writeInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.Errorf("invalid input: %v", err), nil
	}
	if in.Path == "" {
		return tool.Errorf("path is required"), nil
	}
	full := resolve(w.Dir, in.Path)
	if dir := filepath.Dir(full); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return tool.Errorf("cannot create directory: %v", err), nil
		}
	}
	if err := os.WriteFile(full, []byte(in.Content), 0o644); err != nil {
		return tool.Errorf("cannot write %s: %v", in.Path, err), nil
	}
	return tool.Text("wrote " + in.Path + " (" + byteCount(len(in.Content)) + ")"), nil
}
