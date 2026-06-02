package tools

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/brittonhayes/vala/internal/tool"
)

//go:embed ls.md
var lsDescription string

// LS lists directory entries. Read-only.
type LS struct{ Dir string }

func (l *LS) Name() string        { return "ls" }
func (l *LS) Description() string { return lsDescription }
func (l *LS) ReadOnly() bool      { return true }

func (l *LS) Schema() tool.Schema {
	return tool.Schema{
		Properties: map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory to list (default: working directory)."},
		},
	}
}

func (l *LS) Run(_ context.Context, input json.RawMessage) (tool.Result, error) {
	var in struct {
		Path string `json:"path"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return tool.Errorf("invalid input: %v", err), nil
		}
	}
	dir := in.Path
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(resolve(l.Dir, dir))
	if err != nil {
		return tool.Errorf("cannot list %s: %v", dir, err), nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) == 0 {
		return tool.Text("(empty directory)"), nil
	}
	return tool.Text(strings.Join(names, "\n")), nil
}
